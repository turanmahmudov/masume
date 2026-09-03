# Screenshots of the client, for the README, a blog post or a store page.
# Rebuild them with `mise run shots`. They are written to vhs/shots/.
#
# This tape renders larger than vhs/demo.tape and holds each frame only long enough to
# capture it. So it is a screenshot run, not a recording. The mp4 that vhs must write
# goes to .demo, which git does not track.

Output .demo/shots.mp4

Require masume

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 27
Set Width 2560
Set Height 1500
Set Padding 20
Set Framerate 24
Set Theme "ayu"
Set TypingSpeed 25ms

Hide
Type "export XDG_CONFIG_HOME=$PWD/.demo/config XDG_STATE_HOME=$PWD/.demo/state" Enter
Type "clear" Enter
Type "masume" Enter
Sleep 2s
Enter
Sleep 4s
Show

# The tree, unfolded down to a table.
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Right
Sleep 2s
Screenshot vhs/shots/01-object-tree.png

# The rows of that table.
Enter
Sleep 3s
Screenshot vhs/shots/02-grid.png

Type "2" Sleep 2s
Screenshot vhs/shots/03-columns.png

Type "3" Sleep 2s
Screenshot vhs/shots/04-indexes.png

Type "5" Sleep 2s
Screenshot vhs/shots/05-ddl.png

# The object menu, and the diagram that its first entry opens.
Type "m" Sleep 2s
Screenshot vhs/shots/06-object-menu.png

Enter Sleep 3s
Screenshot vhs/shots/07-er-diagram.png
Escape Sleep 800ms

# A partly typed statement with the completion menu open.
Alt+n
Sleep 1500ms
Type "select c.country," Enter
Type "count(*) as orders," Enter
Type "sum(o.total) as revenue" Enter
Type "from orders o" Enter
Type "join cust"
Sleep 2500ms
Screenshot vhs/shots/08-completion.png

Tab
Sleep 700ms
Type " c on c.id = o.customer_id" Enter
Type "group by c.country" Enter
Type "order by revenue desc"
Sleep 1s

Ctrl+r
Sleep 3s
Screenshot vhs/shots/09-result.png

# The plan, with the times that the server measured.
Ctrl+y
Sleep 4s
Screenshot vhs/shots/10-plan.png

# The palette.
Ctrl+k
Sleep 2s
Screenshot vhs/shots/11-palette.png

# And the same screen in a light theme.
Type "theme" Sleep 1500ms
Enter Sleep 1500ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 1500ms
Enter
Sleep 2500ms
Screenshot vhs/shots/12-light-theme.png
Sleep 500ms
