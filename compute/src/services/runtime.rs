//! Runtime call processing service.
use std::sync::Arc;

use grpcio;
use grpcio::{RpcStatus, RpcStatusCode};
use rustracing::tag;
use rustracing_jaeger::span::SpanContext;

use ekiden_compute_api::{Runtime, SubmitTxRequest, SubmitTxResponse};
use ekiden_core::futures::prelude::*;
use ekiden_tracing;
use ekiden_tracing::MetadataCarrier;

use super::super::roothash::{self, RootHashFrontend};

struct RuntimeServiceInner {
    /// Root hash frontend.
    roothash_frontend: Arc<RootHashFrontend>,
}

#[derive(Clone)]
pub struct RuntimeService {
    inner: Arc<RuntimeServiceInner>,
}

impl RuntimeService {
    /// Create new compute server instance.
    pub fn new(roothash_frontend: Arc<RootHashFrontend>) -> Self {
        RuntimeService {
            inner: Arc::new(RuntimeServiceInner { roothash_frontend }),
        }
    }
}

impl Runtime for RuntimeService {
    fn submit_tx(
        &self,
        ctx: grpcio::RpcContext,
        mut request: SubmitTxRequest,
        sink: grpcio::UnarySink<SubmitTxResponse>,
    ) {
        measure_histogram_timer!("submit_tx_time");
        measure_counter_inc!("submit_tx_calls");
        let tracer = ekiden_tracing::get_tracer();
        let mut opts = tracer
            .span("submit_tx")
            .tag(tag::StdTag::span_kind("server"));
        match SpanContext::extract_from_http_header(&MetadataCarrier(ctx.request_headers())) {
            Ok(Some(sc)) => {
                opts = opts.child_of(&sc);
            }
            Ok(None) => {}
            Err(error) => {
                error!(
                    "Tracing provider unable to extract span context: {:?}",
                    error
                );
            }
        }
        let submit_span = opts.start();

        let data = request.take_data();
        let append_span = submit_span.handle().child("append_batch", |opts| {
            opts.tag(tag::StdTag::span_kind("producer")).start()
        });

        let result = match self.inner
            .roothash_frontend
            .append_batch(data, append_span.context().cloned())
        {
            Ok(()) => sink.success(SubmitTxResponse::new()),
            Err(error) => sink.fail(RpcStatus::new(
                match error.description() {
                    roothash::ERROR_APPEND_NOT_LEADER => RpcStatusCode::Unavailable,
                    roothash::ERROR_APPEND_TOO_LARGE => RpcStatusCode::ResourceExhausted,
                    _ => RpcStatusCode::Internal,
                },
                Some(error.description().to_owned()),
            )),
        };

        ctx.spawn(result.map_err(|_error| ()));
    }
}