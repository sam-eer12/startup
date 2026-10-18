//! Synthetic response-transform lab. Signed webhook ingress is a separate Go spike.
use crate::Runtime;
use adapter_core::{Input, MAX_BODY, Output, Plan};
use envoy_types::pb::envoy::{
    config::core::v3::{HeaderValue, HeaderValueOption},
    extensions::filters::http::ext_proc::v3::processing_mode::BodySendMode,
    service::ext_proc::v3::{
        self as pb, processing_request::Request as Message, processing_response::Response as Reply,
    },
    r#type::v3::HttpStatus,
};
use std::{sync::Arc, time::Duration};
use tokio::sync::{Semaphore, mpsc};
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status, Streaming};

pub use pb::external_processor_server::ExternalProcessorServer;

#[derive(Clone)]
pub struct Processor {
    runtime: Arc<Runtime>,
    streams: Arc<Semaphore>,
    executions: Arc<Semaphore>,
}

impl Processor {
    pub fn new(runtime: Runtime) -> Self {
        Self {
            runtime: Arc::new(runtime),
            streams: Arc::new(Semaphore::new(64)),
            executions: Arc::new(Semaphore::new(64)),
        }
    }
}

#[tonic::async_trait]
impl pb::external_processor_server::ExternalProcessor for Processor {
    type ProcessStream = ReceiverStream<Result<pb::ProcessingResponse, Status>>;
    async fn process(
        &self,
        request: Request<Streaming<pb::ProcessingRequest>>,
    ) -> Result<Response<Self::ProcessStream>, Status> {
        let permit = self
            .streams
            .clone()
            .try_acquire_owned()
            .map_err(|_| Status::resource_exhausted("local stream limit"))?;
        let mut stream = request.into_inner();
        let this = self.clone();
        let (tx, rx) = mpsc::channel(1);
        tokio::spawn(async move {
            let _permit = permit;
            let mut state = State::default();
            loop {
                let next = tokio::time::timeout(Duration::from_secs(5), stream.message()).await;
                let message = match next {
                    Ok(Ok(Some(message))) => message,
                    Ok(Ok(None)) => break,
                    _ => {
                        let _ = tx
                            .send(Err(Status::deadline_exceeded(
                                "incomplete processing stream",
                            )))
                            .await;
                        break;
                    }
                };
                let response = this.respond(&mut state, message).await;
                let terminal = matches!(response.response, Some(Reply::ImmediateResponse(_)));
                if !matches!(
                    tokio::time::timeout(Duration::from_millis(25), tx.send(Ok(response))).await,
                    Ok(Ok(()))
                ) || terminal
                {
                    break;
                }
            }
        });
        Ok(Response::new(ReceiverStream::new(rx)))
    }
}

#[derive(Default)]
struct State {
    phase: u8,
    plan: Option<Plan>,
}

impl Processor {
    async fn respond(
        &self,
        state: &mut State,
        request: pb::ProcessingRequest,
    ) -> pb::ProcessingResponse {
        if request.observability_mode {
            return reject(400, "observability_mode_unsupported");
        }
        if let Some(protocol) = request.protocol_config {
            // Streaming/partial bodies are not accepted.
            if protocol.request_body_mode != BodySendMode::None as i32
                || protocol.response_body_mode != BodySendMode::Buffered as i32
                || protocol.send_body_without_waiting_for_header_response
            {
                return reject(400, "buffered_response_required");
            }
        }
        let reply = match request.request {
            Some(Message::RequestHeaders(headers)) if state.phase == 0 => {
                let path = header(&headers, ":path");
                state.plan = match path.split('?').next().unwrap_or("") {
                    "/runtime/identity" => Some(Plan::Identity),
                    "/runtime/move" => Some(Plan::MoveString {
                        from: "/data/object/metadata/presentation/label".into(),
                        path: "/data/object/metadata/display_label".into(),
                    }),
                    _ => return reject(404, "unknown_lab_route"),
                };
                state.phase = 1;
                Reply::RequestHeaders(pb::HeadersResponse {
                    response: Some(pb::CommonResponse {
                        header_mutation: Some(pb::HeaderMutation {
                            remove_headers: vec![
                                "x-mesh-signature".into(),
                                "x-mesh-tenant".into(),
                                "x-mesh-route".into(),
                            ],
                            ..Default::default()
                        }),
                        ..Default::default()
                    }),
                })
            }
            Some(Message::ResponseHeaders(headers)) if state.phase == 1 => {
                if [
                    ":status",
                    "content-type",
                    "content-encoding",
                    "content-length",
                ]
                .iter()
                .any(|key| count_header(&headers, key) > 1)
                {
                    return reject(422, "ambiguous_headers");
                }
                let status = header(&headers, ":status");
                if headers.end_of_stream {
                    state.phase = 3;
                    Reply::ResponseHeaders(pb::HeadersResponse::default())
                } else {
                    if status != "200" {
                        return reject(502, "status_not_eligible");
                    }
                    let media = header(&headers, "content-type");
                    if media.split(';').next().unwrap_or("").trim() != "application/json" {
                        return reject(415, "json_required");
                    }
                    if !matches!(
                        header(&headers, "content-encoding").as_str(),
                        "" | "identity"
                    ) {
                        return reject(415, "encoding_unsupported");
                    }
                    if [
                        "stripe-signature",
                        "x-shopify-hmac-sha256",
                        "x-mesh-signature",
                    ]
                    .iter()
                    .any(|key| count_header(&headers, key) > 0)
                    {
                        return reject(422, "signed_response_requires_trust_workflow");
                    }
                    if let Ok(len) = header(&headers, "content-length").parse::<usize>()
                        && len > MAX_BODY
                    {
                        return reject(413, "body_too_large");
                    }
                    state.phase = 2;
                    let mutation = matches!(state.plan, Some(Plan::MoveString { .. })).then(|| {
                        pb::HeaderMutation {
                            remove_headers: [
                                "content-length",
                                "etag",
                                "content-md5",
                                "digest",
                                "content-digest",
                                "repr-digest",
                                "accept-ranges",
                            ]
                            .map(str::to_string)
                            .into(),
                            ..Default::default()
                        }
                    });
                    Reply::ResponseHeaders(pb::HeadersResponse {
                        response: Some(pb::CommonResponse {
                            header_mutation: mutation,
                            ..Default::default()
                        }),
                    })
                }
            }
            Some(Message::ResponseBody(body)) if state.phase == 2 => {
                if !body.end_of_stream {
                    return reject(422, "incomplete_body");
                }
                if body.body.len() > MAX_BODY {
                    return reject(413, "body_too_large");
                }
                state.phase = 3;
                if body.body.is_empty() {
                    return pb::ProcessingResponse {
                        response: Some(Reply::ResponseBody(pb::BodyResponse::default())),
                        ..Default::default()
                    };
                }
                let Ok(body) = String::from_utf8(body.body) else {
                    return reject(422, "invalid_utf8");
                };
                let input = Input {
                    abi_version: 1,
                    plan: state.plan.clone().unwrap(),
                    body,
                };
                let Ok(permit) = self.executions.clone().try_acquire_owned() else {
                    return reject(503, "executor_busy");
                };
                let runtime = self.runtime.clone();
                let work = tokio::task::spawn_blocking(move || {
                    let _permit = permit;
                    runtime.run(&input)
                });
                let result = tokio::time::timeout(Duration::from_millis(25), work).await;
                let output = match result {
                    Ok(Ok(Ok(output))) => output,
                    _ => return reject(503, "executor_failure"),
                };
                match output {
                    Output::Rejected { .. } => return reject(422, "transform_rejected"),
                    Output::Unchanged { .. } => Reply::ResponseBody(pb::BodyResponse::default()),
                    Output::Transformed { body } => Reply::ResponseBody(pb::BodyResponse {
                        response: Some(pb::CommonResponse {
                            header_mutation: Some(pb::HeaderMutation {
                                set_headers: vec![HeaderValueOption {
                                    header: Some(HeaderValue {
                                        key: "x-mesh-lab-transform".into(),
                                        raw_value: b"move-string-v1".to_vec(),
                                        ..Default::default()
                                    }),
                                    ..Default::default()
                                }],
                                ..Default::default()
                            }),
                            body_mutation: Some(pb::BodyMutation {
                                mutation: Some(pb::body_mutation::Mutation::Body(
                                    body.into_bytes(),
                                )),
                            }),
                            ..Default::default()
                        }),
                    }),
                }
            }
            _ => return reject(400, "invalid_message_sequence"),
        };
        pb::ProcessingResponse {
            response: Some(reply),
            ..Default::default()
        }
    }
}

fn header(headers: &pb::HttpHeaders, key: &str) -> String {
    let Some(map) = &headers.headers else {
        return String::new();
    };
    let values: Vec<_> = map.headers.iter().filter(|h| h.key == key).collect();
    if values.len() != 1 {
        return String::new();
    }
    let h = values[0];
    if h.raw_value.is_empty() {
        h.value.clone()
    } else {
        String::from_utf8_lossy(&h.raw_value).into_owned()
    }
}

fn count_header(headers: &pb::HttpHeaders, key: &str) -> usize {
    headers
        .headers
        .as_ref()
        .map(|map| map.headers.iter().filter(|h| h.key == key).count())
        .unwrap_or(0)
}

fn reject(code: i32, detail: &str) -> pb::ProcessingResponse {
    pb::ProcessingResponse {
        response: Some(Reply::ImmediateResponse(pb::ImmediateResponse {
            status: Some(HttpStatus { code }),
            body: b"local runtime rejected body".to_vec(),
            details: detail.into(),
            ..Default::default()
        })),
        ..Default::default()
    }
}
