# CLI demo tape for the schedularr README. Regenerated automatically by
# .github/workflows/demo.yaml whenever this directory changes; run locally
# with `vhs vhs/cli.tape` from the repo root after `make web-presence &&
# go build -o bin/schedularr .`.
#
# Only deterministic, side-effect-free commands are shown here: --help
# output and validating a fixture that already ships in testdata/. Nothing
# in this tape touches a live Tunarr instance or writes files outside the
# terminal recording itself.

Output assets/cli.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 600
Set Theme "Dracula"
Set Padding 20
Set TypingSpeed 50ms

# Alias setup happens off-camera so the recording opens straight on the
# first real command.
Hide
Type "alias schedularr=./bin/schedularr"
Enter
Type "clear"
Enter
Show

Type "schedularr --help"
Sleep 750ms
Enter
Sleep 4s

Type "schedularr validate testdata/configs/valid_scheduler.yaml"
Sleep 750ms
Enter
Sleep 2500ms
