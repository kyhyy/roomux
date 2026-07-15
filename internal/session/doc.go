// Package session tracks sessions, windows, and who is present in them.
//
// maintain the registry of active sessions and
// their windows, record which verified users are attached (presence), and back
// the `roomux who` listing and the attach/detach/window-switch history log.
package session
