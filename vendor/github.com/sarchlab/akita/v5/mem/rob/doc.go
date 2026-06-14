// Package rob implements a reorder buffer that preserves the order in which
// memory responses are returned to the requester. The component forwards each
// incoming request from its Top port to a configured bottom unit through its
// Bottom port, tracks the resulting transactions in FIFO order, and releases
// responses to the Top port only when the head-of-line transaction has
// completed.
package rob
