package domain

// RoutingDecision is an immutable value object produced by the Routing Engine.
// It tells the pipeline which connector to use and why.
//
// Once created, no pipeline stage may modify it.
type RoutingDecision struct {
	RouteID          string
	ConnectorID      string
	StrategyUsed     string   // static, round_robin, failover, weighted
	Priority         int
	Cost             int64    // thousandths of a cent, at selection time
	Reason           string   // why this route was chosen
	CapabilitiesUsed []string
}
