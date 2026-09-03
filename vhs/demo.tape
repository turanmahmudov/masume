# The README recording. Rebuild it with `mise run demo`. That task starts the postgres
# container, loads vhs/seed.sql into it, and runs this tape.
#
# vhs needs ttyd and ffmpeg on the PATH. See https://github.com/charmbracelet/vhs.
# The glyphs in vhs/config.toml need a Nerd Font, so FontFamily below is required.
# Width, Height and FontSize scale together. Raising all three keeps the same character
# grid and renders it with more pixels.

Output vhs/demo.gif

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
Type "export XDG_CONFIG_HOME=$PWD/.demo/config XDG_STATE_HOME=$PWD/.demo/state" Enter
Type "clear" Enter
Show

Type "masume" Sleep 500ms Enter
Sleep 2s

# Pick the connection.
Enter
Sleep 3s

# Unfold a table in the tree. It shows the columns, with the primary and foreign keys marked.
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Right
Sleep 2s

# Open the table. Each table view is one key press away.
Enter
Sleep 2500ms
Type "2" Sleep 2200ms
Type "3" Sleep 2200ms
Type "4" Sleep 2200ms
Type "5" Sleep 2500ms

# The object menu, and the ER diagram of the tables related to this one.
Type "m"
Sleep 2200ms
Enter
Sleep 3500ms
Escape
Sleep 800ms

# A new tab opens with the caret already in the editor.
Alt+n
Sleep 1200ms
Type "select c.country," Enter
Type "count(*) as orders," Enter
Type "sum(o.total) as revenue" Enter

# The completion menu suggests names from the catalog. Tab accepts the highlighted name.
Type "from ord"
Sleep 2200ms
Tab
Sleep 700ms
Type " o" Enter
Type "join cust"
Sleep 2200ms
Tab
Sleep 700ms
Type " c on c.id = o.customer_id" Enter
Type "group by c.country" Enter
Type "order by revenue desc"
Sleep 1200ms

# Run it.
Ctrl+r
Sleep 2500ms

# The plan, first estimated and then measured.
Ctrl+e
Sleep 3s
Ctrl+y
Sleep 3500ms

# The command palette searches actions by name.
Ctrl+k
Sleep 1200ms
Type "theme"
Sleep 1200ms
Enter
Sleep 1500ms

# Moving the cursor repaints the whole screen in the highlighted theme before you choose one.
# Tokyo Night, Catppuccin Mocha, Gruvbox Dark, Dracula, Nord.
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1400ms

# The light themes come after Rosé Pine and Solarized Dark. Stop on Catppuccin Latte.
Down Sleep 500ms
Down Sleep 500ms
Down Sleep 2500ms

# Esc cancels the preview, and the screen returns to the theme from the config file.
Escape
Sleep 2s
