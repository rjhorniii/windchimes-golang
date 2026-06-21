# Wind Chimes for Linux (Go)

A Linux reimplementation of **Syntrillium Wind Chimes 1.01** in pure Go, with no external dependencies. Simulates real wind-chime pendulum physics — each chime swings under randomized wind gusts using a pink noise source, and notes trigger when two pendulums collide (exactly how real chimes work).

---

## How It Works

The program:
1. Simulates **pendulum physics** for each chime — wind applies random forces, gravity restores them
2. Detects **chime collisions** (when two pendulums swing close together) and fires MIDI notes
3. Outputs **raw MIDI bytes** to stdout (or a file/device)

## Requirements

- Go 1.18+ 
- A MIDI synthesizer on Linux, one of:
  - **TiMidity++**
  - **FluidSynth**
  - **Hardware MIDI** 


Windchimes outputs raw MIDI bytes to stdout, which are piped to fluidsynth using a Linux virtual midi port.  

## Added Requirements when using the windchimes.sh script

- fluidsynth
- Linux kernel module
  - `snd-virmidi`

## Installation
To build and run:
```bash
go build -o windchimes windchimes.go
```
Windchimes is controlled by a terminal interface.  It starts running a preset configuration automatically.  You can make interactive keyboard control changes to wind speed, scale, instrument, base note, number of chimes, or randomize everything, while it plays. Wind physics use a pink random noise generator to control wind speed, gusts, etc. The wind speed affects both note frequency and velocity. In high winds it add solo strums for realism.  (Pink noise is used instead of uniform random because pink noise gives a more realistic wind behavior.)

- 12 named presets including Soft Ambiance, Japanese Garden, Storm, Gunfight!, Meditation, etc. 
- 19 scales including pentatonic, blues, hirajoshi, whole tone, chromatic, etc.
- 29 GM instruments including tubular bells, marimba, vibraphone, koto, sitar, steel drums, gunshot, etc.

The original software required Windows 95 and a SoundBlaster. This one just needs normal Linux alsa or pipewire sound. Windchimes is based upon the original user manual and reviews of the original program.  It is not derived from original code or executable.

Windchimes is a standalone go-lang program.  The bash script `windchimes.sh` needs to be edited to reflect your local installation and setup preferences. In particular these lines near the top need to be localized to reference your chosen location and selection for soundfont and windchimes executable.  You may want to change the choice of virtual midi port.
```bash
SOUNDFONT="/home/xxxxx/Downloads/GeneralUser-GS.sf2"
VIRMIDI_PORT_NAME="Virtual Raw MIDI 0-0"   # adjust if you want a different virmidi port
VIRMIDI_DEVICE="/dev/snd/midiC0D0"         # what windchimes writes to
WINDCHIMES_BIN="./windchimes"              # adjust path if run from elsewhere
```
The script assumes that the virtual midi module has been loaded into the kernel.  This might be either by manually using `modprobe snd-virmidi` or by modifying the `modules-load` to automatically load this upon system boot.
