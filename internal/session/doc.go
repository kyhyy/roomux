// Package session is not used yet.
//
// It will track the sessions, the windows of each session, and who is present
// in them, when roomux has more than one session. Today there is one session,
// and the daemon owns it and its windows. This package will then hold the
// registry of active sessions, record which verified users are attached
// (presence), and back the `roomux who` listing and the attach/detach/switch
// history log.
package session
