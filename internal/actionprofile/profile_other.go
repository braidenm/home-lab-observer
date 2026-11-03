//go:build !linux

package actionprofile

import "syscall"

func VerifyHostNamespace() error                       { return ErrProfile }
func VerifyWorker() (Identity, error)                  { return Identity{}, ErrProfile }
func VerifyBroker(uint32) (Identity, error)            { return Identity{}, ErrProfile }
func VerifyAdmin() (Identity, error)                   { return Identity{}, ErrProfile }
func VerifyWorkerPeer(syscall.RawConn, Identity) error { return ErrProfile }
func VerifyBrokerPeer(syscall.RawConn, Identity) error { return ErrProfile }
func VerifyAdminPeer(syscall.RawConn, Identity) error  { return ErrProfile }
