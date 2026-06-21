# Wind Chimes for Linux (Go)

A Linux reimplementation of **Syntrillium Wind Chimes 1.01** in pure Go, with no external dependencies. Simulates real wind-chime pendulum physics — each chime swings under randomized wind gusts using a pink noise source, and notes trigger when two pendulums collide (exactly how real chimes work).

---

## How It Works

The program:
1. Simulates **pendulum physics** for each chime — wind applies random forces, gravity restores them
2. Detects **chime collisions** (when two pendulums swing close together) and fires MIDI notes
3. Outputs **raw MIDI bytes** to stdout (or a file/device)

## Requirements

- Go 1.18+ (`apt install golang-go` on Ubuntu/Debian)
- A MIDI synthesizer on Linux — one of:
  - **TiMidity++**
  - **FluidSynth**
  - **Hardware MIDI**

## Build

```bash
go build -o windchimes windchimes.go
```

## Run

```bash
# With TiMidity (most common):
./windchimes | timidity -

# With FluidSynth:
./windchimes | fluidsynth -a alsa -m alsa-raw -

# Write to hardware MIDI port:
./windchimes --out /dev/snd/midiC0D0

# Start with a specific preset:
./windchimes --preset 5 | timidity -

# Override wind speed:
./windchimes --preset 2 --wind 0.7 | timidity -

# List all presets:
./windchimes --list-presets
```

## Interactive Controls

Type a command and press Enter while the program is running:

| Key | Action |
|-----|--------|
| `+` / `-` | Wind speed ±10% |
| `++` / `--` | Wind speed ±25% |
| `n` / `p` | Next / previous scale |
| `i` / `I` | Next / previous instrument |
| `b` / `B` | Base note up / down one semitone |
| `c` / `C` | More / fewer chimes |
| `r` | Randomize all settings |
| `l` | List presets |
| `1`–`9`, `0` | Load preset 1–10 |
| `s` | Show current settings |
| `?` | Help |
| `q` | Quit |

## Presets

| # | Name | Wind | Scale |
|---|------|------|-------|
| 1 | Soft Ambiance | 15% | New Pentatonic |
| 2 | Gentle Breeze | 20% | New Pentatonic |
| 3 | Wind in the Trees | 35% | Major |
| 4 | Storm | 85% | New Pentatonic |
| 5 | Japanese Garden | 20% | Hirajoshi |
| 6 | Glorious Sunrise | 30% | Major |
| 7 | Cathedral | 25% | Lydian |
| 8 | Blues Alley | 40% | Blues |
| 9 | Steel Drum Party | 60% | New Pentatonic |
| 0 | Gunfight! | 95% | Chromatic |
| (11) | Twilight | 18% | Dorian |
| (12) | Meditation | 10% | Hirajoshi |

## Scales Available

New Pentatonic, Pentatonic Minor, Major, Natural Minor, Dorian, Mixolydian, Lydian, Phrygian, Locrian, Harmonic Minor, Whole Tone, Diminished, Blues, Chromatic, Hirajoshi, Pelog, Arabian, Hungarian Minor, Octatonic.

## Instruments Available

Tubular Bells, Marimba, Vibraphone, Xylophone, Glockenspiel, Music Box, Celesta, Church Organ, Acoustic Guitar, Electric Piano, Harpsichord, Dulcimer, Koto, Sitar, Steel Drums, Orchestral Harp, Pads, Crystal, Gunshot, and more.

## Physics Notes

- Each chime is a damped pendulum driven by stochastic wind gusts
- Wind speed scales both the force magnitude and gust frequency
- Chimes ring when two pendulums swing close together with sufficient relative velocity
- Note velocity (loudness) maps to the relative collision speed
- Cooldown prevents the same chime ringing too rapidly
- At high wind speeds, solo "strums" are added for realism

## Differences from Original

- MIDI output via stdout (pipe to any synth) rather than direct Windows MIDI API
- All 19 scales vs. the original's 43 presets (but presets cover the same range of moods)
- No Kaleidoscope screensaver integration (Linux only)
- No nag screen 😄
