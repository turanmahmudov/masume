# The notebook recording. Rebuild it with `mise run demo-notebook`. That task starts the
# postgres container, loads vhs/seed.sql into it, copies vhs/notebook.masume.md into the
# notebook directory of the demo, and runs this tape.
#
# vhs needs ttyd and ffmpeg on the PATH. See https://github.com/charmbracelet/vhs.
# The glyphs in vhs/config.toml need a Nerd Font, so FontFamily below is required.

Output vhs/notebook.mp4

Require masume

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 19
Set Width 1800
Set Height 1060
Set Padding 14
Set Framerate 24
Set Theme "ayu"
Set TypingSpeed 40ms

Hide
Type "export XDG_CONFIG_HOME=$PWD/.demo/config XDG_STATE_HOME=$PWD/.demo/state MASUME_DEMO_PASSWORD=secret" Enter
Type "clear" Enter
Show

Type "masume" Sleep 500ms Enter
Sleep 2s

# Pick the connection.
Enter
Sleep 2500ms

# The notebooks card lists the notebooks of the project and of the user.
Alt+o
Type "n"
Sleep 2500ms
Enter
Sleep 2500ms

# Opening a notebook runs no cell. Every cell reads `not run`.
Sleep 1500ms

# Alt+R runs every cell, in order, one result each. The parameter cell binds :status for
# every statement below it.
Alt+r
Sleep 4s

# The list moves with j and k. The result pane draws the result of the cell it stands on.
Type "j" Sleep 1200ms
Type "j" Sleep 2500ms

# A chart cell draws the result of the cell it names.
Type "j" Sleep 3s

# Enter opens the form of a chart: the source cell, the two columns, the shape and the
# order. The columns offered are the columns that cell returned, and Enter applies it.
Enter
Sleep 2500ms
Down Sleep 600ms
Down Sleep 600ms
Right Sleep 2000ms
Left Sleep 1200ms
Down Sleep 800ms
Right Sleep 2500ms
Enter
Sleep 3s

# The last cell, and the run of one cell on its own.
Type "j" Sleep 1200ms
Type "r"
Sleep 3s

# A new cell asks for its kind first: statements, prose, values, or a chart.
Type "b"
Sleep 2500ms
Down Sleep 700ms
Down Sleep 1500ms
Escape
Sleep 1200ms

# The whole notebook is one Markdown file. Ctrl+P writes it.
Ctrl+p
Sleep 2500ms
Escape
Sleep 1500ms

# Every key of the cell list is in the help, under one scope of its own.
Type "?"
Sleep 1500ms
Type "cell"
Sleep 3500ms
Escape
Sleep 1500ms
