// Package grpcclient provides shared gRPC client connection helpers for
// adapter-layer transports.
//
// These helpers verify TCP/transport readiness after grpc.NewClient without
// invoking business RPCs or the gRPC Health Checking Protocol.
package grpcclient
