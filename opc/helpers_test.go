package opc_test

import "io"

type readCloser = io.ReadCloser

type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }
