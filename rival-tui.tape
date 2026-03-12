Output rival-tui.gif

Set Shell "zsh"
Set Width 1400
Set Height 700
Set FontSize 15
Set FontFamily "Cascadia Code NF"
Set Theme "Catppuccin Mocha"
Set Padding 20
Set WindowBar Colorful
Set WindowBarSize 40
Set TypingSpeed 80ms

# Launch TUI
Type "rival tui"
Enter
Sleep 2s

# Navigate — first one should be running
Sleep 500ms
Down
Sleep 500ms
Down
Sleep 500ms

# Back to top (running session)
Up
Sleep 300ms
Up
Sleep 500ms

# Open detail view of running session
Enter
Sleep 3s

# Back to list
Escape
Sleep 800ms

# Open a completed session
Down
Sleep 300ms
Down
Sleep 300ms
Enter
Sleep 2s

# Back and quit
Escape
Sleep 500ms
Type "q"
Sleep 500ms
