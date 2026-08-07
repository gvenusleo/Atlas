# atlas_acp

Atlas's Agent Client Protocol server adapter.

This package maps ACP sessions and updates to the shared runtime. It must not duplicate agent orchestration or persistence.

ACP owns its JSON-RPC lifecycle and uses `json_rpc_2` directly instead of depending on a shared Atlas RPC wrapper.
