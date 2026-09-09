// Package identity finds the real identity of a client.
//
// The kernel records the user, the group and the process of the peer when it
// connects to a Unix domain socket. The client cannot change these values.
// Therefore the daemon does not have to trust a name that the client sends.
package identity
