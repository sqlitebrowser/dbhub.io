## go-nonblockingchan

[![Build Status](https://travis-ci.org/hectane/go-nonblockingchan.svg?branch=master)](https://travis-ci.org/hectane/go-nonblockingchan)
[![GoDoc](https://godoc.org/github.com/hectane/go-nonblockingchan?status.svg)](https://godoc.org/github.com/hectane/go-nonblockingchan)
[![MIT License](http://img.shields.io/badge/license-MIT-9370d8.svg?style=flat)](http://opensource.org/licenses/MIT)

A special type that mimics the behavior of a channel but does not block when items are sent.

### Features

- Send items without ever worrying that the send will block
- Check how many items are waiting to be received
- Synchronized access to members - use it from any goroutine

### Usage

To use the package, add the following import:

    import "github.com/hectane/go-nonblockingchan"

Use the `New()` function to create a new instance:

    c := nbc.New()

To send an item on the channel, use the `Send` field:

    c.Send <- true

Sending will always succeed immediately. The item will be added to an internal buffer until it is received:

    v, ok := <-c.Recv
    if ok {
        // value was received
    } else {
        // channel was closed
    }
