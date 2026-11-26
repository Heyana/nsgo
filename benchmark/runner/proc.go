package main

import "errors"

var errProcess = errors.New("cannot read the process")

// pidSet is the processes running when a program starts, so those it starts can be told from them; nil where the runner does not look for them.
type pidSet map[int32]bool
