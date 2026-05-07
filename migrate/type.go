package migrate

type State string

const (
	StateRunning    State = "running"
	StateApplied    State = "applied"
	StateFailed     State = "failed"
	StateRolledBack State = "rolled_back"
)
