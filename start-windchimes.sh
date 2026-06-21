#!/bin/bash
#
# Starts FluidSynth, connects it to a VirMIDI port, then starts windchimes
# in the foreground so you can type commands to it.
#
# Prerequisite (one-time, not in this script):
#   echo snd-virmidi >> /etc/modules-load.d/virmidi.conf
# so the module is loaded at boot without needing sudo here.

set -u

SOUNDFONT="/home/xxxxx/Downloads/GeneralUser-GS.sf2"
VIRMIDI_PORT_NAME="Virtual Raw MIDI 0-0"   # adjust if you want a different virmidi port
VIRMIDI_DEVICE="/dev/snd/midiC0D0"         # what windchimes writes to
WINDCHIMES_BIN="./windchimes"              # adjust path if run from elsewhere

# --- Sanity checks -----------------------------------------------------

if [ ! -f "$SOUNDFONT" ]; then
  echo "Soundfont not found at $SOUNDFONT — aborting."
  exit 1
fi

if ! lsmod | grep -q snd_virmidi; then
  echo "snd-virmidi not loaded. Run: sudo modprobe snd-virmidi"
  exit 1
fi

# --- Start FluidSynth ----------------------------------------------------

# -a pipewire: routes audio through PipeWire so it shows up in pavucontrol
# -s / --server: run as a server, doesn't need an interactive stdin shell
fluidsynth -a pipewire -m alsa_seq -g 1.5 -s -i "$SOUNDFONT"  &
FLUID_PID=$!

cleanup() {
  echo "Shutting down FluidSynth (PID $FLUID_PID)..."
  kill "$FLUID_PID" 2>/dev/null
}
trap cleanup EXIT

FLUID_CLIENT=""
for i in $(seq 1 20); do
  FLUID_CLIENT=$(aconnect -o | grep -B1 "FLUID Synth" | grep client | head -1 | sed -E 's/client ([0-9]+).*/\1/')
  [ -n "$FLUID_CLIENT" ] && break
  sleep 0.5
done

if [ -z "$FLUID_CLIENT" ]; then
  echo "FluidSynth client never appeared — aborting."
  exit 1
fi

# --- Find the VirMIDI client ----------------------------------------------

VIRMIDI_CLIENT=$(aconnect -i | grep -A1 "$VIRMIDI_PORT_NAME" | grep client | head -1 | sed -E 's/client ([0-9]+).*/\1/')

if [ -z "$VIRMIDI_CLIENT" ]; then
  echo "VirMIDI client '$VIRMIDI_PORT_NAME' not found — is snd-virmidi loaded?"
  exit 1
fi

# --- Connect VirMIDI -> FluidSynth ----------------------------------------

aconnect "$VIRMIDI_CLIENT" "$FLUID_CLIENT"
echo "Connected VirMIDI client $VIRMIDI_CLIENT to FluidSynth client $FLUID_CLIENT"

# --- Start windchimes in the foreground -------------------------------
#
# Runs attached to the terminal so stdin works for interactive commands.
# Script blocks here until windchimes exits; the trap then cleans up fluidsynth.

"$WINDCHIMES_BIN" --out "$VIRMIDI_DEVICE"
