use std::env;

use axum::{
    http::header,
    response::IntoResponse,
    routing::get,
    Router,
};

async fn root() -> impl IntoResponse {
    ([(header::CONTENT_TYPE, "text/plain")], "hello\n")
}

async fn hello() -> impl IntoResponse {
    (
        [(header::CONTENT_TYPE, "application/json")],
        "{\"msg\":\"hello from rust-axum\"}",
    )
}

#[tokio::main]
async fn main() {
    let port: u16 = env::var("PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let app = Router::new()
        .route("/", get(root))
        .route("/api/hello", get(hello));

    let addr = format!("0.0.0.0:{port}");
    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
