// Package approvals implements strict 4-eyes production deploys (access-control P2): the deploy gate
// (config-selected ApprovalPolicy), approval requests, decisions, comments and notifications.
// An approval is a deployments row in pending_approval; activation goes through routes' Activator port.
package approvals
