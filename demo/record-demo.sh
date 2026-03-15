#!/usr/bin/env bash
# Record an asciinema demo of cc-setup using fake config data.
# Produces demo.cast in the demo/ directory.
#
# The demo shows:
#   1. Server list with health checks, inherited servers, and toggle
#   2. Scope switching (project vs user)
#   3. Detail view and tool permissions
#   4. Plugin management with unsaved-changes dialog on exit
#
# State management constraints:
#   - Scope toggles reload checked state from disk (loses in-memory changes)
#   - Returning from the detail view recreates the manage model (fresh state)
#   - So scope toggle and detail view must happen BEFORE the final toggle
#   - Plugin toggle is done last to trigger the unsaved-changes dialog on quit

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CAST_FILE="$SCRIPT_DIR/demo.cast"
FIXTURE_DIR=$(mktemp -d)
TMUX_SESSION="cc-setup-demo"
COLS=100
ROWS=28

# ── Prerequisites ────────────────────────────────────────────────────────────

for cmd in asciinema tmux cc-setup npx; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Error: $cmd is not installed."
    case "$cmd" in
      cc-setup)  echo "Install with: brew install cc-deck/tap/cc-setup" ;;
      asciinema) echo "Install with: brew install asciinema" ;;
      tmux)      echo "Install with: brew install tmux" ;;
      npx)       echo "Install with: brew install node" ;;
    esac
    exit 1
  fi
done

# ── Pre-cache npx package ───────────────────────────────────────────────────
# The everything server is used for the tool permissions demo.
# Pre-caching avoids a long download delay during recording.

echo "Pre-caching @modelcontextprotocol/server-everything..."
npx -y @modelcontextprotocol/server-everything --help >/dev/null 2>&1 || true

# ── Generate fixtures ────────────────────────────────────────────────────────

echo "Generating demo fixtures..."
bash "$SCRIPT_DIR/create-fixtures.sh" "$FIXTURE_DIR" > /dev/null

# ── Prepare recording environment ───────────────────────────────────────────

DEMO_BASHRC=$(mktemp)
cat > "$DEMO_BASHRC" <<BASHRC
PS1='\[\033[1;32m\]\$ \[\033[0m\]'
export HOME=$FIXTURE_DIR/home
export XDG_CONFIG_HOME=$FIXTURE_DIR/home/.config
cd $FIXTURE_DIR/workspace/my-project
BASHRC

rm -f "$CAST_FILE"
tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
tmux new-session -d -s "$TMUX_SESSION" -x "$COLS" -y "$ROWS"

echo "Recording demo..."

# Start asciinema recording with a clean bash shell
tmux send-keys -t "$TMUX_SESSION" \
  "asciinema rec --cols $COLS --rows $ROWS --overwrite '$CAST_FILE' -c 'bash --rcfile $DEMO_BASHRC --noprofile -i'" Enter

sleep 1.5

# Clear screen for a fresh start
tmux send-keys -t "$TMUX_SESSION" "clear" Enter
sleep 0.5

# ── Act 1: Server List & Navigation ─────────────────────────────────────────
# Shows: health check dots, inherited servers with muted styling, toggle

# Launch cc-setup
tmux send-keys -t "$TMUX_SESSION" "cc-setup" Enter

# Wait for health checks to settle (everything server needs ~3s via npx)
sleep 5

# Navigate down through all servers to show the full list
# Alphabetical order: confluence(0), everything(1), filesystem(2), github(3),
#   google-workspace(4), jira(5), kubernetes(6), slack(7), sqlite-db(8)
for i in 1 2 3 4 5 6 7 8; do
  tmux send-keys -t "$TMUX_SESSION" j
  sleep 0.4
done

sleep 1

# Navigate up to google-workspace (from index 8 to 4)
for i in 1 2 3 4; do
  tmux send-keys -t "$TMUX_SESSION" k
  sleep 0.3
done

sleep 0.5

# Toggle google-workspace ON to show the toggle feature
tmux send-keys -t "$TMUX_SESSION" " "
sleep 1.5

# ── Act 2: Scope Toggle ─────────────────────────────────────────────────────
# Shows: switching between project and user scope
# Note: scope toggle reloads state from disk, losing the toggle above.
# That's fine visually: the user already saw the toggle.

# Switch to User scope
tmux send-keys -t "$TMUX_SESSION" "."
sleep 2

# Switch back to Project scope
tmux send-keys -t "$TMUX_SESSION" "."
sleep 1.5

# ── Act 3: Detail View & Tool Permissions ────────────────────────────────────
# Shows: server details, tool discovery, permission toggles
# Note: returning from detail view restarts the manage model with fresh state.

# Navigate to everything server (cursor is at 4=google-workspace after scope toggle)
# Go up from 4 to 1 (everything)
for i in 1 2 3; do
  tmux send-keys -t "$TMUX_SESSION" k
  sleep 0.3
done
sleep 0.3

# Enter detail view
tmux send-keys -t "$TMUX_SESSION" e
sleep 1.5

# Navigate down to "Tool permissions" (last field, index 6)
# Fields: Type(0), Source(1), Command(2), Arguments(3), Environment(4),
#   Description(5), Tool permissions(6)
for i in 1 2 3 4 5 6; do
  tmux send-keys -t "$TMUX_SESSION" j
  sleep 0.3
done

sleep 0.5

# Press enter to launch tool discovery
tmux send-keys -t "$TMUX_SESSION" Enter

# Wait for tool discovery to complete and TUI to start.
# The everything server starts via npx (~3-5s) then tool listing (~1-2s).
# Measured at ~6.5s in previous recording, using 9s for safety margin.
sleep 9

# In tool permissions screen: toggle first tool, move down, toggle another
tmux send-keys -t "$TMUX_SESSION" " "
sleep 0.5
tmux send-keys -t "$TMUX_SESSION" j
sleep 0.3
tmux send-keys -t "$TMUX_SESSION" j
sleep 0.3
tmux send-keys -t "$TMUX_SESSION" " "
sleep 1.5

# Press q to cancel tool permissions (back to detail view)
tmux send-keys -t "$TMUX_SESSION" q
sleep 1.5

# Press q to go back to main list (manage model restarts with fresh state)
tmux send-keys -t "$TMUX_SESSION" q
sleep 1.5

# ── Act 4: Plugins & Exit ───────────────────────────────────────────────────
# Shows: plugin tab, toggling a plugin, unsaved-changes dialog
# The plugin toggle is done LAST (no scope toggle or detail view after)
# so the unsaved change is preserved when quit is pressed.

# Switch to Plugins tab
tmux send-keys -t "$TMUX_SESSION" Tab
sleep 1

# Navigate down to prose plugin
# Sorted: copyedit(0), jira(1), kubernetes(2), prose(3), sdd(4)
for i in 1 2 3; do
  tmux send-keys -t "$TMUX_SESSION" j
  sleep 0.3
done

sleep 0.5

# Toggle prose ON (was disabled in project scope)
tmux send-keys -t "$TMUX_SESSION" " "
sleep 1.5

# Quit: triggers unsaved-changes dialog (prose was toggled)
tmux send-keys -t "$TMUX_SESSION" q
sleep 1.5

# Discard changes and quit
tmux send-keys -t "$TMUX_SESSION" d
sleep 1

# Exit the recorded shell (ends asciinema recording)
tmux send-keys -t "$TMUX_SESSION" "exit" Enter
sleep 1

# ── Cleanup ──────────────────────────────────────────────────────────────────

tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
rm -rf "$FIXTURE_DIR" "$DEMO_BASHRC"

if [ -f "$CAST_FILE" ]; then
  echo "Recording saved to $CAST_FILE"
  echo "Run ./export-gif.sh to convert to GIF"
else
  echo "Error: Recording failed, $CAST_FILE not found"
  exit 1
fi
