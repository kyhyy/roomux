# roomux

A minimal terminal multiplexer (a tmux alternative) with native multi-user
identity. When several people attach to a shared session, each participant's
identity is verified through kernel peer credentials (`SO_PEERCRED`) rather than
being self-reported, giving trustworthy presence and history. roomux is a single
Go binary with a client-server architecture over a Unix domain socket, spawning
shells via a pty.

## Status

Working for one shared session with several windows. The daemon spawns a shell in
a pty for each window and streams it live to the clients that view that window;
`who` lists who is attached now and in which window, and `history` lists the
attach, detach, and window-switch events. Identity comes from the kernel.

Not built yet: more than one session, a live status line, and a history that
survives a daemon restart.

## Build and run

```
make build

./bin/roomux daemon      # terminal 1 — start the daemon and the first window
./bin/roomux attach      # terminal 2 — join the session; Ctrl+Space opens the
                         #   command bar: new | next | prev | select N | detach
./bin/roomux who         # who is attached right now, and in which window
./bin/roomux history     # the attach, detach, and switch events
```

The socket lives in `/tmp` and is world-writable, so any local user can attach.
That is intended. The daemon reads each client's real uid through the kernel, so
the identity shown by `who` and `history` is trustworthy even though the socket
is open.
