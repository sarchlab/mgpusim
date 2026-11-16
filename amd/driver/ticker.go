package driver

// Ticker schedules Driver after a complete host round.
type Ticker interface{ TickLater() }
