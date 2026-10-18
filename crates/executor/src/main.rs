use anyhow::{Result, ensure};
use executor::{
    Runtime,
    ext_proc::{ExternalProcessorServer, Processor},
};

#[tokio::main]
async fn main() -> Result<()> {
    let mut args = std::env::args().skip(1);
    let artifact = args
        .next()
        .unwrap_or_else(|| "target/wasm32-unknown-unknown/release/adapter_wasm.wasm".into());
    let address = args
        .next()
        .unwrap_or_else(|| "127.0.0.1:50051".into())
        .parse::<std::net::SocketAddr>()?;
    ensure!(
        address.ip().is_loopback()
            || std::env::var("MESH_CONTAINER_LOCAL").as_deref() == Ok("synthetic-only"),
        "loopback or explicit synthetic container profile required"
    );
    let processor = Processor::new(Runtime::new(&std::fs::read(artifact)?)?);
    eprintln!("synthetic ext_proc listening on {address}");
    tonic::transport::Server::builder()
        .add_service(
            ExternalProcessorServer::new(processor)
                .max_decoding_message_size(300 * 1024)
                .max_encoding_message_size(300 * 1024),
        )
        .serve_with_shutdown(address, async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await?;
    Ok(())
}
