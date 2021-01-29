//! Registry structures.
//!
//! # Note
//!
//! This **MUST** be kept in sync with go/registry/api.
//!
use serde::{Deserialize, Serialize};
use serde_repr::*;

use crate::{
    common::{
        crypto::{
            hash::Hash,
            signature::{PublicKey, SignatureBundle},
        },
        namespace::Namespace,
        quantity,
        version::Version,
    },
    consensus::staking,
    storage::mkvs::WriteLog,
};
use std::{collections::BTreeMap, time::Duration};

#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize_repr, Deserialize_repr)]
#[repr(u32)]
pub enum RuntimeKind {
    #[serde(rename = "invalid")]
    KindInvalid = 0,
    #[serde(rename = "compute")]
    KindCompute = 1,
    #[serde(rename = "keymanager")]
    KindKeyManager = 2,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct ExecutorParameters {
    pub group_size: u64,
    pub group_backup_size: u64,
    pub allowed_stragglers: u64,
    pub round_timeout: i64,
    pub max_messages: u32,
    pub min_pool_size: u64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct TxnSchedulerParameters {
    pub algorithm: String,
    pub batch_flush_timeout: Duration,
    pub max_batch_size: u64,
    pub max_batch_size_bytes: u64,
    pub propose_batch_timeout: i64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct StorageParameters {
    pub group_size: u64,
    pub min_write_replication: u64,
    pub max_apply_write_log_entries: u64,
    pub max_apply_ops: u64,
    pub checkpoint_interval: u64,
    pub checkpoint_num_kept: u64,
    pub checkpoint_chunk_size: u64,
    pub min_pool_size: u64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct RuntimeStakingParameters {
    pub thresholds: Option<BTreeMap<staking::ThresholdKind, quantity::Quantity>>,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize_repr, Deserialize_repr)]
#[repr(u32)]
pub enum RolesMask {
    #[serde(rename = "compute")]
    RoleComputeWorker = 1 << 0,
    #[serde(rename = "storage")]
    RoleStorageWorker = 1 << 1,
    #[serde(rename = "key-manager")]
    RoleKeyManager = 1 << 2,
    #[serde(rename = "validator")]
    RoleValidator = 1 << 3,
    #[serde(rename = "consensus-rpc")]
    RoleConsensusRPC = 1 << 4,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct EntityWhitelistRuntimeAdmissionPolicy {
    #[serde(rename = "entities")]
    pub entities: Option<BTreeMap<PublicKey, EntityWhitelistConfig>>,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct EntityWhitelistConfig {
    #[serde(rename = "max_nodes")]
    pub max_nodes: Option<BTreeMap<RolesMask, u16>>,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum RuntimeAdmissionPolicy {
    #[serde(rename = "any_node")]
    AnyNode(),
    #[serde(rename = "entity_whitelist")]
    EntityWhitelist(EntityWhitelistRuntimeAdmissionPolicy),
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize_repr, Deserialize_repr)]
#[repr(u8)]
pub enum RuntimeGovernanceModel {
    #[serde(rename = "invalid")]
    GovernanceInvalid = 0,
    #[serde(rename = "entity")]
    GovernanceEntity = 1,
    #[serde(rename = "runtime")]
    GovernanceRuntime = 2,
    #[serde(rename = "consensus")]
    GovernanceConsensus = 3,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct VersionInfo {
    pub version: Version,
    pub tee: Option<Vec<u8>>,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize_repr, Deserialize_repr)]
#[repr(u8)]
pub enum TEEHardware {
    #[serde(rename = "invalid")]
    TEEHardwareInvalid = 0,
    #[serde(rename = "intel-sgx")]
    TEEHardwareIntelSGX = 1,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct Runtime {
    pub v: u16,
    pub id: Namespace,
    pub entity_id: PublicKey,
    pub genesis: RuntimeGenesis,
    pub kind: RuntimeKind,
    pub tee_hardware: TEEHardware,
    pub versions: VersionInfo,
    pub key_manager: Option<Namespace>,
    pub executor: Option<ExecutorParameters>,
    pub txn_scheduler: Option<TxnSchedulerParameters>,
    pub storage: Option<StorageParameters>,
    pub admission_policy: RuntimeAdmissionPolicy,
    pub staking: Option<RuntimeStakingParameters>,
    pub governance_model: RuntimeGovernanceModel,
}

/// Runtime genesis information that is used to initialize runtime state in the first block.
#[derive(Clone, Debug, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct RuntimeGenesis {
    /// State root that should be used at genesis time. If the runtime should start with empty state,
    /// this must be set to the empty hash.
    pub state_root: Hash,

    /// State identified by the state_root. It may be empty iff all storage_receipts are valid or
    /// state_root is an empty hash or if used in network genesis (e.g. during consensus chain init).
    pub state: WriteLog,

    /// Storage receipts for the state root. The list may be empty or a signature in the list
    /// invalid iff the state is non-empty or state_root is an empty hash or if used in network
    /// genesis (e.g. during consensus chain init).
    pub storage_receipts: Vec<SignatureBundle>,

    /// Runtime round in the genesis.
    pub round: u64,
}
