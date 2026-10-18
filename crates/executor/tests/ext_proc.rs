use envoy_types::pb::envoy::{
    config::core::v3::{HeaderMap, HeaderValue},
    service::ext_proc::v3::{
        self as pb, processing_request::Request as Message, processing_response::Response as Reply,
    },
};
use executor::{
    Runtime,
    ext_proc::{ExternalProcessorServer, Processor},
};
use tokio_stream::wrappers::TcpListenerStream;

fn headers(items: &[(&str, &str)], end: bool) -> pb::HttpHeaders {
    pb::HttpHeaders {
        headers: Some(HeaderMap {
            headers: items
                .iter()
                .map(|(key, value)| HeaderValue {
                    key: (*key).into(),
                    raw_value: value.as_bytes().to_vec(),
                    ..Default::default()
                })
                .collect(),
        }),
        end_of_stream: end,
        ..Default::default()
    }
}
fn message(request: Message) -> pb::ProcessingRequest {
    pb::ProcessingRequest {
        protocol_config: matches!(&request, Message::RequestHeaders(_)).then_some(
            pb::ProtocolConfiguration {
                request_body_mode: 0,
                response_body_mode: 2,
                send_body_without_waiting_for_header_response: false,
            },
        ),
        request: Some(request),
        ..Default::default()
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn real_http2_concurrent_streams_and_body_policy() {
    let wasm = std::fs::read("../../target/wasm32-unknown-unknown/release/adapter_wasm.wasm")
        .expect("make wasm");
    let processor = Processor::new(Runtime::new(&wasm).unwrap());
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let (stop, stopped) = tokio::sync::oneshot::channel();
    let server = tokio::spawn(
        tonic::transport::Server::builder()
            .add_service(ExternalProcessorServer::new(processor))
            .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                let _ = stopped.await;
            }),
    );
    let client = pb::external_processor_client::ExternalProcessorClient::connect(format!(
        "http://{address}"
    ))
    .await
    .unwrap();
    let body = include_bytes!("../../../fixtures/synthetic/stripe/field-move.json");
    let mut tasks = Vec::new();
    for _ in 0..4 {
        let mut client = client.clone();
        tasks.push(tokio::spawn(async move {
            let requests = vec![
                message(Message::RequestHeaders(headers(
                    &[(":path", "/runtime/move")],
                    true,
                ))),
                message(Message::ResponseHeaders(headers(
                    &[
                        (":status", "200"),
                        ("content-type", "application/json"),
                        ("etag", "stale"),
                    ],
                    false,
                ))),
                message(Message::ResponseBody(pb::HttpBody {
                    body: body.to_vec(),
                    end_of_stream: true,
                    ..Default::default()
                })),
            ];
            let mut stream = client
                .process(tokio_stream::iter(requests))
                .await
                .unwrap()
                .into_inner();
            let mut responses = Vec::new();
            while let Some(response) = stream.message().await.unwrap() {
                responses.push(response);
            }
            assert_eq!(responses.len(), 3);
            let Some(Reply::ResponseHeaders(header)) = &responses[1].response else {
                panic!("unexpected header response")
            };
            assert!(
                header
                    .response
                    .as_ref()
                    .unwrap()
                    .header_mutation
                    .as_ref()
                    .unwrap()
                    .remove_headers
                    .contains(&"etag".into())
            );
            let Some(Reply::ResponseBody(result)) = &responses[2].response else {
                panic!("unexpected body response: {:?}", responses[2])
            };
            let mutation = &result
                .response
                .as_ref()
                .unwrap()
                .body_mutation
                .as_ref()
                .unwrap()
                .mutation;
            let Some(pb::body_mutation::Mutation::Body(bytes)) = mutation else {
                panic!("missing replacement body")
            };
            let value: serde_json::Value = serde_json::from_slice(bytes).unwrap();
            assert_eq!(
                value["data"]["object"]["metadata"]["display_label"],
                "Local demo"
            );
            assert_eq!(value["id"], "evt_synthetic_001");
        }));
    }
    for task in tasks {
        task.await.unwrap();
    }
    for (raw, end, encoding, status) in [
        (b"{".to_vec(), true, "", 422),
        (body.to_vec(), false, "", 422),
        (body.to_vec(), true, "gzip", 415),
        (vec![b' '; 262145], true, "", 413),
    ] {
        let mut client = client.clone();
        let requests = vec![
            message(Message::RequestHeaders(headers(
                &[(":path", "/runtime/identity")],
                true,
            ))),
            message(Message::ResponseHeaders(headers(
                &[
                    (":status", "200"),
                    ("content-type", "application/json"),
                    ("content-encoding", encoding),
                ],
                false,
            ))),
            message(Message::ResponseBody(pb::HttpBody {
                body: raw,
                end_of_stream: end,
                ..Default::default()
            })),
        ];
        let mut stream = client
            .process(tokio_stream::iter(requests))
            .await
            .unwrap()
            .into_inner();
        let mut rejection = None;
        while let Some(response) = stream.message().await.unwrap() {
            if let Some(Reply::ImmediateResponse(response)) = response.response {
                rejection = response.status.map(|status| status.code);
            }
        }
        assert_eq!(rejection, Some(status));
    }
    let mut client = client.clone();
    let requests = vec![
        message(Message::RequestHeaders(headers(
            &[(":path", "/runtime/move")],
            true,
        ))),
        message(Message::ResponseHeaders(headers(
            &[(":status", "204")],
            true,
        ))),
    ];
    let mut stream = client
        .process(tokio_stream::iter(requests))
        .await
        .unwrap()
        .into_inner();
    assert!(stream.message().await.unwrap().is_some());
    assert!(matches!(
        stream.message().await.unwrap().unwrap().response,
        Some(Reply::ResponseHeaders(_))
    ));
    stop.send(()).unwrap();
    drop(stream);
    server.await.unwrap().unwrap();
}
