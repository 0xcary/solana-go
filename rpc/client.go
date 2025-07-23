// Copyright 2021 github.com/gagliardetto
// This file has been modified by github.com/gagliardetto
//
// Copyright 2020 dfuse Platform Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/klauspost/compress/gzhttp"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrNotConfirmed = errors.New("not confirmed")
)

type Client struct {
	rpcURL    string
	rpcClient JSONRPCClient
	headerFn  atomic.Value
}

type JSONRPCClient interface {
	CallForInto(ctx context.Context, out interface{}, method string, params []interface{}) error
	CallWithCallback(ctx context.Context, method string, params []interface{}, callback func(*http.Request, *http.Response) error) error
	CallBatch(ctx context.Context, requests jsonrpc.RPCRequests) (jsonrpc.RPCResponses, error)
}

// New creates a new Solana JSON RPC client.
// Client is safe for concurrent use by multiple goroutines.
func New(rpcEndpoint string) *Client {
	opts := &jsonrpc.RPCClientOpts{
		HTTPClient: newHTTP(),
	}

	rpcClient := jsonrpc.NewClientWithOpts(rpcEndpoint, opts)
	return NewWithCustomRPCClient(rpcClient)
}

// New creates a new Solana JSON RPC client with the provided custom headers.
// The provided headers will be added to each RPC request sent via this RPC client.
func NewWithHeaders(rpcEndpoint string, headers map[string]string) *Client {
	opts := &jsonrpc.RPCClientOpts{
		HTTPClient:    newHTTP(),
		CustomHeaders: headers,
	}
	rpcClient := jsonrpc.NewClientWithOpts(rpcEndpoint, opts)
	return NewWithCustomRPCClient(rpcClient)
}

// Close closes the client.
func (cl *Client) Close() error {
	if cl.rpcClient == nil {
		return nil
	}
	if c, ok := cl.rpcClient.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// NewWithCustomRPCClient creates a new Solana RPC client
// with the provided RPC client.
func NewWithCustomRPCClient(rpcClient JSONRPCClient) *Client {
	return &Client{
		rpcClient: rpcClient,
	}
}

var (
	defaultMaxIdleConnsPerHost = 9
	defaultTimeout             = 5 * time.Minute
	defaultKeepAlive           = 180 * time.Second
)

func newHTTPTransport() *http.Transport {
	return &http.Transport{
		IdleConnTimeout:     defaultTimeout,
		MaxConnsPerHost:     defaultMaxIdleConnsPerHost,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		Proxy:               http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultTimeout,
			KeepAlive: defaultKeepAlive,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2: true,
		// MaxIdleConns:          100,
		TLSHandshakeTimeout: 10 * time.Second,
		// ExpectContinueTimeout: 1 * time.Second,
	}
}

// newHTTP returns a new Client from the provided config.
// Client is safe for concurrent use by multiple goroutines.
func newHTTP() *http.Client {
	tr := newHTTPTransport()

	return &http.Client{
		Timeout:   defaultTimeout,
		Transport: gzhttp.Transport(tr),
	}
}

// RPCCallForInto allows to access the raw RPC client and send custom requests.
func (cl *Client) RPCCallForInto(ctx context.Context, out interface{}, method string, params []interface{}) error {
	return cl.rpcClient.CallForInto(ctx, out, method, params)
}

func (cl *Client) RPCCallWithCallback(
	ctx context.Context,
	method string,
	params []interface{},
	callback func(*http.Request, *http.Response) error,
) error {
	return cl.rpcClient.CallWithCallback(ctx, method, params, callback)
}

func (cl *Client) RPCCallBatch(
	ctx context.Context,
	requests jsonrpc.RPCRequests,
) (jsonrpc.RPCResponses, error) {
	return cl.rpcClient.CallBatch(ctx, requests)
}

func NewBoolean(b bool) *bool {
	return &b
}

func NewTransactionVersion(v uint64) *uint64 {
	return &v
}

type HeaderRoundTripper struct {
	Base    http.RoundTripper
	Headers func() map[string]string
}

func (h *HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Headers() {
		req.Header.Set(k, v)
	}
	return h.Base.RoundTrip(req)
}

func newHTTPWithHeaderFn(headerFn func() map[string]string) *http.Client {
	baseTransport := newHTTPTransport()

	transport := &HeaderRoundTripper{
		Base:    gzhttp.Transport(baseTransport),
		Headers: headerFn,
	}

	return &http.Client{
		Timeout:   defaultTimeout,
		Transport: transport,
	}
}

func NewWithDynamicHeaders(rpcEndpoint string) *Client {
	client := &Client{}

	// default header function is nil
	// it will be set later by the user.
	client.headerFn.Store(func() map[string]string {
		return map[string]string{}
	})

	httpClient := newHTTPWithHeaderFn(func() map[string]string {
		if v := client.headerFn.Load(); v != nil {
			if f, ok := v.(func() map[string]string); ok {
				return f()
			}
		}
		return nil
	})

	rpcClient := jsonrpc.NewClientWithOpts(rpcEndpoint, &jsonrpc.RPCClientOpts{
		HTTPClient: httpClient,
	})
	client.rpcClient = rpcClient
	return client
}

func (cl *Client) SetHeaderFunc(f func() map[string]string) {
	cl.headerFn.Store(f)
}
