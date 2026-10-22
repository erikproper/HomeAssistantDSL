---
name: project-frame-vienna-xserver-systemd-supervision-2026-10-19
description: "DONE+VERIFIED 2026-10-19: Vienna's picture-frame X server/feh slideshow now runs under a systemd unit (xinit-slideshow.service, Restart=always) instead of an unsupervised background process -- fixes a real incident where it silently sat on a bare text login screen for hours after X exited. Applied identically to Junglinster's frame 2026-10-20 (verified)."
metadata:
  node_type: memory
  type: project
  modified: 2026-10-19T05:52:38.672Z
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
---

**Incident**: Vienna's picture-frame display (`frame.vienna`) is supposed to go dark (not just
dim) when not showing photos, via `xrandr --output HDMI-1 --off` + `kill -19` (SIGSTOP) on `feh`
-- `stop_slideshow`/`start_slideshow`/`check_slideshow` (`/home/erikp/bin/`), driven by
`mqtt_commandline`'s "slideshow" switch. This "off" mechanism never touches the X server itself
-- X was expected to just keep running indefinitely underneath. Found live 2026-10-19: X had
exited (clean exit, `Xorg.0.log`: "Server terminated successfully (0)") and nothing noticed or
relaunched it, so the display spent HOURS showing the bare `agetty` text login prompt instead
-- a real risk for the monitor, and the opposite of "off".

**Root cause not fully confirmed, but doesn't need to be** -- leading theory from the log: ~17
minutes before the clean exit, the `vc4-hdmi` input device (the virtual input tied to the HDMI
connection's own hotplug/CEC channel) was removed, right before X terminated. Plausible
mechanism: the monitor's own standby, once it sees no signal (xrandr has disabled the output),
drops HDMI hotplug-detect on its end, which can cascade into X losing its only input device and
deciding to exit. A `320x200` framebuffer reallocation seen in the log around the same time
turned out to be a RED HERRING -- confirmed live afterward that this is just X's normal "no
active output mode" screen size whenever `xrandr --output HDMI-1 --off` is in effect, not a sign
of anything unusual.

**Fix: supervise X with systemd, independent of the on/off business logic.** New
`/etc/systemd/system/xinit-slideshow.service` (`Restart=always`, `RestartSec=2`,
`ExecStart=/usr/bin/xinit /home/erikp/bin/run_slideshow -- -s off -dpms -s noblank` -- the exact
invocation `start_slideshow` used to run manually), enabled at boot via
`multi-user.target.wants`. Whatever the real trigger is, X now comes back within ~2 seconds of
any exit instead of sitting dead for hours -- verified live by force-killing Xorg directly
(`sudo pkill -9 -f /usr/lib/xorg/Xorg`) and confirming systemd relaunched it (new PID, feh
running fresh) within the RestartSec window.

**`start_slideshow` simplified, same session**: its old manual recovery branch (`kill -9` any
stray `xinit`/`Xorg`/`feh`, then manually relaunch via `sudo xinit ... &`) is now dead code AND a
race risk -- if X happens to be mid-respawn (systemd already relaunching it) at the exact moment
`start_slideshow` also runs its own "no feh PID found" check, both could try to spawn a second X
server fighting over the same VT. Removed that branch entirely; `start_slideshow` now ONLY does
`xrandr --output HDMI-1 --auto` + resume `feh` if paused, trusting systemd to keep X alive
underneath. `stop_slideshow`/`check_slideshow` unchanged (they never touched X's own lifecycle
anyway). Verified both still work correctly against the systemd-supervised X (resolution/feh
process state toggle correctly both directions).

**Junglinster's frame, 2026-10-20**: had the identical pre-fix setup (Ubuntu 26.04, X launched
by hand via `sudo xinit ... &`, old `start_slideshow` with the kill-and-relaunch branch). Applied the
same unit file and simplified `start_slideshow` verbatim (old script kept as
`~/bin/start_slideshow.pre-systemd-2026-10-20`). Verified the same way: `kill -9` Xorg → new Xorg + feh
within RestartSec; start/stop/check toggle correctly. Left in the "off" state it was in before.
Gotcha hit while doing it: a remote `ssh ... 'pkill -f "sudo xinit /home/erikp/..."'` matches the
ssh session's OWN `bash -c` command line and kills the session mid-script -- use `pkill -x` or
`kill <pid>` instead.

**How to apply**: if this symptom (bare text login screen showing on a picture-frame display)
recurs on EITHER house after this fix, the systemd unit itself masks the symptom (X always comes
back) but the underlying HDMI-hotplug/input-loss theory is still unconfirmed -- worth actually
chasing down if it keeps recurring frequently enough to matter, rather than just leaning on
auto-respawn forever.
