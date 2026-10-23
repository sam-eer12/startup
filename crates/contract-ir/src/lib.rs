//! Bounded contract normalization and local drift diagnostics. Observations never
//! become authoritative contracts and this crate cannot authorize a repair.
pub mod detect;
pub mod schema;
pub mod wire;
use serde::{Deserialize, Serialize};

/// Observed samples cannot be converted into a consumer binding implicitly.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ContractAuthority {
    SyntheticFixture,
    OwnerApproved,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ConsumerContract {
    pub schema_version: u32,
    pub contract_id: String,
    pub contract_version: u32,
    pub tenant_id: String,
    pub environment_id: String,
    pub authority: ContractAuthority,
    pub owner: String,
    pub dialect: String,
    pub schema: serde_json::Value,
}
