//! Consensus staking structures.
//!
//! # Note
//!
//! This **MUST** be kept in sync with go/staking/api.
//!
use serde::{Deserialize, Serialize};
use serde_repr::*;

use crate::{common::quantity::Quantity, consensus::address::Address};

/// A stake transfer.
#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct Transfer {
    pub to: Address,
    pub amount: Quantity,
}

/// A withdrawal from an account.
#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct Withdraw {
    pub from: Address,
    pub amount: Quantity,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize_repr, Deserialize_repr)]
#[repr(i32)]
pub enum ThresholdKind {
    #[serde(rename = "entity")]
    KindEntity = 0,
    #[serde(rename = "node-validator")]
    KindNodeValidator = 1,
    #[serde(rename = "node-compute")]
    KindNodeCompute = 2,
    #[serde(rename = "node-storage")]
    KindNodeStorage = 3,
    #[serde(rename = "node-keymanager")]
    KindNodeKeyManager = 4,
    #[serde(rename = "runtime-compute")]
    KindRuntimeCompute = 5,
    #[serde(rename = "runtime-keymanager")]
    KindRuntimeKeyManager = 6,
}
