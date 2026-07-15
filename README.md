# roomux

A minimal terminal multiplexer (a tmux alternative) with native multi-user
identity. When several people attach to a shared session, each participant's
identity is verified through kernel peer credentials (`SO_PEERCRED`) rather than
being self-reported, giving trustworthy presence and history. roomux is a single
Go binary with a client-server architecture over a Unix domain socket, spawning
shells via a pty.

**Status:** early scaffolding, not usable yet.
