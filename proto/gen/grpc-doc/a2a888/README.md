# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [a2a888/agent_runtime.proto](#a2a888_agent_runtime-proto)
    - [CacheIdentity](#a2a888-v1-CacheIdentity)
    - [CompatibilityEvidence](#a2a888-v1-CompatibilityEvidence)
    - [CompatibilityReport](#a2a888-v1-CompatibilityReport)
    - [CustomRuntimeConfig](#a2a888-v1-CustomRuntimeConfig)
    - [EmbeddedRuntimeConfig](#a2a888-v1-EmbeddedRuntimeConfig)
    - [NpmPackageConfig](#a2a888-v1-NpmPackageConfig)
    - [PermissionProfile](#a2a888-v1-PermissionProfile)
    - [PlatformTarget](#a2a888-v1-PlatformTarget)
    - [PreparedRuntime](#a2a888-v1-PreparedRuntime)
    - [ProviderCapabilities](#a2a888-v1-ProviderCapabilities)
    - [ProviderManifest](#a2a888-v1-ProviderManifest)
    - [ResolvedBinary](#a2a888-v1-ResolvedBinary)
    - [RuntimeStatus](#a2a888-v1-RuntimeStatus)
    - [SessionBehavior](#a2a888-v1-SessionBehavior)
    - [SystemExecutableConfig](#a2a888-v1-SystemExecutableConfig)

    - [AgentProtocol](#a2a888-v1-AgentProtocol)
    - [CompatibilityLevel](#a2a888-v1-CompatibilityLevel)
    - [RuntimeKind](#a2a888-v1-RuntimeKind)
    - [RuntimeState](#a2a888-v1-RuntimeState)
    - [SessionMode](#a2a888-v1-SessionMode)

- [Scalar Value Types](#scalar-value-types)



<a name="a2a888_agent_runtime-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## a2a888/agent_runtime.proto



<a name="a2a888-v1-CacheIdentity"></a>

### CacheIdentity
CacheIdentity is content-addressed and must not change for the lifetime of a
prepared runtime. A changed manifest or artifact creates a new identity.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identity_digest | [string](#string) |  |  |
| provider_id | [string](#string) |  |  |
| manifest_digest | [string](#string) |  |  |
| runtime_version | [string](#string) |  |  |
| platform | [PlatformTarget](#a2a888-v1-PlatformTarget) |  |  |
| lock_digest | [string](#string) |  |  |
| package_name | [string](#string) |  |  |
| package_version | [string](#string) |  |  |
| integrity | [string](#string) |  | SRI integrity value for package-backed runtimes. |






<a name="a2a888-v1-CompatibilityEvidence"></a>

### CompatibilityEvidence
CompatibilityEvidence is an auditable result from a detection or test run.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| version | [string](#string) |  |  |
| platform | [PlatformTarget](#a2a888-v1-PlatformTarget) |  |  |
| tested_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| details | [string](#string) |  |  |






<a name="a2a888-v1-CompatibilityReport"></a>

### CompatibilityReport
CompatibilityReport records the strongest verified compatibility level and
the evidence that supports it.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| level | [CompatibilityLevel](#a2a888-v1-CompatibilityLevel) |  |  |
| evidence | [CompatibilityEvidence](#a2a888-v1-CompatibilityEvidence) | repeated |  |






<a name="a2a888-v1-CustomRuntimeConfig"></a>

### CustomRuntimeConfig
CustomRuntimeConfig describes a provider-specific launch command.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  |  |
| arguments | [string](#string) | repeated |  |
| inherited_environment | [string](#string) | repeated | Names of host environment variables that may be inherited; values are supplied by the runtime host and are never stored in the manifest. |
| version | [string](#string) |  |  |
| integrity_sha256 | [string](#string) |  |  |






<a name="a2a888-v1-EmbeddedRuntimeConfig"></a>

### EmbeddedRuntimeConfig
EmbeddedRuntimeConfig identifies a runtime shipped with the host service.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| artifact | [string](#string) |  |  |
| version | [string](#string) |  |  |
| binary | [string](#string) |  |  |
| integrity_sha256 | [string](#string) |  |  |






<a name="a2a888-v1-NpmPackageConfig"></a>

### NpmPackageConfig
NpmPackageConfig pins every value needed to resolve an npm runtime.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| package_name | [string](#string) |  |  |
| package_version | [string](#string) |  |  |
| binary | [string](#string) |  |  |
| integrity | [string](#string) |  | Subresource Integrity (SRI) value for the exact package artifact. |
| registry | [string](#string) |  |  |
| arguments | [string](#string) | repeated |  |






<a name="a2a888-v1-PermissionProfile"></a>

### PermissionProfile
PermissionProfile records the least-privilege permissions requested by a
runtime. Empty allowlists mean that the corresponding access is denied.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_execution | [bool](#bool) |  |  |
| inherit_environment | [bool](#bool) |  |  |
| filesystem_read_paths | [string](#string) | repeated |  |
| filesystem_write_paths | [string](#string) | repeated |  |
| network_hosts | [string](#string) | repeated |  |
| network_access | [bool](#bool) |  |  |






<a name="a2a888-v1-PlatformTarget"></a>

### PlatformTarget
PlatformTarget uses normalized runtime platform names, such as linux,
darwin, windows, amd64, arm64, and glibc.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operating_system | [string](#string) |  |  |
| architecture | [string](#string) |  |  |
| libc | [string](#string) |  |  |
| variant | [string](#string) |  |  |






<a name="a2a888-v1-PreparedRuntime"></a>

### PreparedRuntime
PreparedRuntime is the immutable result of resolving and verifying a
ProviderManifest for one platform.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| provider_id | [string](#string) |  |  |
| cache_identity | [CacheIdentity](#a2a888-v1-CacheIdentity) |  |  |
| resolved_binary | [ResolvedBinary](#a2a888-v1-ResolvedBinary) |  |  |
| status | [RuntimeStatus](#a2a888-v1-RuntimeStatus) |  |  |
| compatibility | [CompatibilityReport](#a2a888-v1-CompatibilityReport) |  |  |
| prepared_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="a2a888-v1-ProviderCapabilities"></a>

### ProviderCapabilities
ProviderCapabilities is a typed capability declaration for runtime
discovery and session orchestration.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| model_discovery | [bool](#bool) |  |  |
| session_resume | [bool](#bool) |  |  |
| streaming | [bool](#bool) |  |  |
| steering | [bool](#bool) |  |  |
| mcp | [bool](#bool) |  |  |
| tool_traces | [bool](#bool) |  |  |






<a name="a2a888-v1-ProviderManifest"></a>

### ProviderManifest
ProviderManifest describes how a provider runtime is identified, installed,
started, and authorized on a supported platform.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| provider_id | [string](#string) |  | Stable provider identifier. Provider IDs are intentionally strings so that providers can be added without changing this contract. |
| display_name | [string](#string) |  |  |
| runtime_kind | [RuntimeKind](#a2a888-v1-RuntimeKind) |  |  |
| agent_protocol | [AgentProtocol](#a2a888-v1-AgentProtocol) |  |  |
| platform_targets | [PlatformTarget](#a2a888-v1-PlatformTarget) | repeated |  |
| system_executable | [SystemExecutableConfig](#a2a888-v1-SystemExecutableConfig) |  |  |
| npm_package | [NpmPackageConfig](#a2a888-v1-NpmPackageConfig) |  |  |
| embedded | [EmbeddedRuntimeConfig](#a2a888-v1-EmbeddedRuntimeConfig) |  |  |
| custom | [CustomRuntimeConfig](#a2a888-v1-CustomRuntimeConfig) |  |  |
| capabilities | [ProviderCapabilities](#a2a888-v1-ProviderCapabilities) |  |  |
| permission_profile | [PermissionProfile](#a2a888-v1-PermissionProfile) |  |  |
| session_behavior | [SessionBehavior](#a2a888-v1-SessionBehavior) |  |  |
| manifest_version | [string](#string) |  |  |
| manifest_integrity_sha256 | [string](#string) |  |  |






<a name="a2a888-v1-ResolvedBinary"></a>

### ResolvedBinary
ResolvedBinary identifies the exact executable selected for a prepared
runtime, including its content digest.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| binary | [string](#string) |  |  |
| version | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |
| size_bytes | [uint64](#uint64) |  |  |
| source | [string](#string) |  |  |
| arguments | [string](#string) | repeated |  |






<a name="a2a888-v1-RuntimeStatus"></a>

### RuntimeStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| state | [RuntimeState](#a2a888-v1-RuntimeState) |  |  |
| message | [string](#string) |  |  |
| observed_version | [string](#string) |  |  |
| observed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| failure_code | [string](#string) |  |  |






<a name="a2a888-v1-SessionBehavior"></a>

### SessionBehavior
SessionBehavior defines lifecycle and resume expectations for provider
sessions.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mode | [SessionMode](#a2a888-v1-SessionMode) |  |  |
| supports_resume | [bool](#bool) |  |  |
| supports_concurrent_sessions | [bool](#bool) |  |  |
| idle_timeout_seconds | [uint32](#uint32) |  |  |
| requires_clean_shutdown | [bool](#bool) |  |  |






<a name="a2a888-v1-SystemExecutableConfig"></a>

### SystemExecutableConfig
SystemExecutableConfig identifies an executable already installed or
provided by the host operating system.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| executable | [string](#string) |  |  |
| arguments | [string](#string) | repeated |  |
| version_argument | [string](#string) |  |  |
| version_pattern | [string](#string) |  |  |
| package_manager | [string](#string) |  |  |
| package_name | [string](#string) |  |  |
| package_version | [string](#string) |  |  |
| integrity_sha256 | [string](#string) |  |  |
| inherited_environment | [string](#string) | repeated | Names of host environment variables that may be inherited; values are supplied by the runtime host and are never stored in the manifest. |








<a name="a2a888-v1-AgentProtocol"></a>

### AgentProtocol


| Name | Number | Description |
| ---- | ------ | ----------- |
| AGENT_PROTOCOL_UNSPECIFIED | 0 |  |
| ACP_V1 | 1 |  |
| ACP_V2 | 2 |  |
| A2A | 3 |  |
| CUSTOM_PROTOCOL | 4 |  |



<a name="a2a888-v1-CompatibilityLevel"></a>

### CompatibilityLevel


| Name | Number | Description |
| ---- | ------ | ----------- |
| COMPATIBILITY_LEVEL_UNSPECIFIED | 0 |  |
| DETECTED | 1 |  |
| PROTOCOL_READY | 2 |  |
| FUNCTIONALLY_VERIFIED | 3 |  |
| FULL_LOOP_VERIFIED | 4 |  |



<a name="a2a888-v1-RuntimeKind"></a>

### RuntimeKind


| Name | Number | Description |
| ---- | ------ | ----------- |
| RUNTIME_KIND_UNSPECIFIED | 0 |  |
| SYSTEM_EXECUTABLE | 1 |  |
| NPM_PACKAGE | 2 |  |
| EMBEDDED | 3 |  |
| CUSTOM_RUNTIME | 4 |  |



<a name="a2a888-v1-RuntimeState"></a>

### RuntimeState


| Name | Number | Description |
| ---- | ------ | ----------- |
| RUNTIME_STATE_UNSPECIFIED | 0 |  |
| UNAVAILABLE | 1 |  |
| INSTALLING | 2 |  |
| VERIFYING | 3 |  |
| READY | 4 |  |
| BROKEN | 5 |  |
| QUARANTINED | 6 |  |
| UPDATE_AVAILABLE | 7 |  |



<a name="a2a888-v1-SessionMode"></a>

### SessionMode


| Name | Number | Description |
| ---- | ------ | ----------- |
| SESSION_MODE_UNSPECIFIED | 0 |  |
| EPHEMERAL | 1 |  |
| PERSISTENT | 2 |  |










## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |
