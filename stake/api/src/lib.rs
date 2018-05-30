extern crate futures;
extern crate grpcio;
extern crate protobuf;

extern crate ekiden_common_api;

mod generated;

use ekiden_common_api as common;

pub use generated::stake::*;
pub use generated::stake_grpc::*;