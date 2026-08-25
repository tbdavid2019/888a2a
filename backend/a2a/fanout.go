package a2a

// JoinPolicy defines the success criteria for aggregating fan-out tasks.
type JoinPolicy string

const (
	JoinPolicyAllSuccess     JoinPolicy = "ALL_SUCCESS"
	JoinPolicyPartialFailure JoinPolicy = "PARTIAL_FAILURE"
	JoinPolicyQuorum         JoinPolicy = "QUORUM"
	JoinPolicyFirstSuccess   JoinPolicy = "FIRST_SUCCESS"
)
