// windchimes - A Linux reimplementation of Syntrillium Wind Chimes
//
// Outputs raw MIDI to stdout. Pipe to a software synth:
//   ./windchimes | timidity -
//   ./windchimes | fluidsynth -a alsa -m alsa-raw -
//   ./windchimes > /dev/midi0     (if you have a hardware MIDI port)
//
// Or write to an ALSA sequencer using aconnect:
//   ./windchimes --port hw:0,0
//
// Interactive controls (press keys while running, then Enter):
//   +/-       Increase/decrease wind speed
//   n         Cycle to next scale
//   p         Cycle to previous scale
//   i         Cycle to next MIDI instrument
//   b         Step base note up by semitone
//   B         Step base note down by semitone
//   c/C       More/fewer chimes
//   r         Randomize settings
//   l         List presets
//   1..9      Load preset by number
//   s         Show current settings
//   q         Quit

package main

import (
	"bufio"
	"flag"
	"fmt"
//	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// MIDI helpers (raw MIDI bytes, no external deps)
// ---------------------------------------------------------------------------

type MIDIWriter struct {
	mu  sync.Mutex
	out *bufio.Writer
}

func NewMIDIWriter(f *os.File) *MIDIWriter {
	return &MIDIWriter{out: bufio.NewWriter(f)}
}

func (m *MIDIWriter) write(b ...byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.out.Write(b)
	m.out.Flush()
}

func (m *MIDIWriter) NoteOn(ch, note, vel byte) {
	if vel == 0 {
		vel = 1
	}
	m.write(0x90|ch, note&0x7F, vel&0x7F)
}

func (m *MIDIWriter) NoteOff(ch, note byte) {
	m.write(0x80|ch, note&0x7F, 0x40)
}

func (m *MIDIWriter) ProgramChange(ch, prog byte) {
	m.write(0xC0|ch, prog&0x7F)
}

func (m *MIDIWriter) ControlChange(ch, cc, val byte) {
	m.write(0xB0|ch, cc&0x7F, val&0x7F)
}

func (m *MIDIWriter) AllNotesOff(ch byte) {
	m.ControlChange(ch, 123, 0)
}

// ---------------------------------------------------------------------------
// Scales
// ---------------------------------------------------------------------------

type Scale struct {
	Name      string
	Intervals []int // semitone offsets from root
}

var Scales = []Scale{
	{"New Pentatonic", []int{0, 2, 4, 7, 9}},
	{"Pentatonic Minor", []int{0, 3, 5, 7, 10}},
	{"Major (Ionian)", []int{0, 2, 4, 5, 7, 9, 11}},
	{"Natural Minor (Aeolian)", []int{0, 2, 3, 5, 7, 8, 10}},
	{"Dorian", []int{0, 2, 3, 5, 7, 9, 10}},
	{"Mixolydian", []int{0, 2, 4, 5, 7, 9, 10}},
	{"Lydian", []int{0, 2, 4, 6, 7, 9, 11}},
	{"Phrygian", []int{0, 1, 3, 5, 7, 8, 10}},
	{"Locrian", []int{0, 1, 3, 5, 6, 8, 10}},
	{"Harmonic Minor", []int{0, 2, 3, 5, 7, 8, 11}},
	{"Whole Tone", []int{0, 2, 4, 6, 8, 10}},
	{"Diminished", []int{0, 2, 3, 5, 6, 8, 9, 11}},
	{"Blues", []int{0, 3, 5, 6, 7, 10}},
	{"Chromatic", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
	{"Hirajoshi (Japanese)", []int{0, 2, 3, 7, 8}},
	{"Pelog (Balinese)", []int{0, 1, 3, 7, 8}},
	{"Arabian", []int{0, 2, 4, 5, 6, 8, 10}},
	{"Hungarian Minor", []int{0, 2, 3, 6, 7, 8, 11}},
	{"Octatonic", []int{0, 1, 3, 4, 6, 7, 9, 10}},
}

// Build a set of MIDI note numbers spanning ~3 octaves from baseNote
func buildNoteSet(baseNote int, scale Scale, numChimes int) []int {
	notes := []int{}
	for octave := 0; octave <= 3; octave++ {
		for _, interval := range scale.Intervals {
			n := baseNote + octave*12 + interval
			if n >= 21 && n <= 108 {
				notes = append(notes, n)
			}
		}
	}
	// Limit to numChimes distinct pitches if fewer requested
	if len(notes) > numChimes*3 {
		notes = notes[:numChimes*3]
	}
	return notes
}

// ---------------------------------------------------------------------------
// Instruments (a curated selection from GM)
// ---------------------------------------------------------------------------

type Instrument struct {
	Name string
	PC   byte // 0-indexed GM program number
}

var Instruments = []Instrument{
	{"Tubular Bells", 14},
	{"Marimba", 12},
	{"Vibraphone", 11},
	{"Xylophone", 13},
	{"Glockenspiel", 9},
	{"Music Box", 10},
	{"Celesta", 8},
	{"Church Organ", 19},
	{"Acoustic Guitar (nylon)", 24},
	{"Acoustic Guitar (steel)", 25},
	{"Electric Piano 1", 4},
	{"Electric Piano 2", 5},
	{"Harpsichord", 6},
	{"Clavinet", 7},
	{"Dulcimer", 15},
	{"Koto", 107},
	{"Sitar", 104},
	{"Banjo", 105},
	{"Shamisen", 106},
	{"Steel Drums", 114},
	{"Woodblock", 115},
	{"Orchestral Harp", 46},
	{"Pad 1 (new age)", 88},
	{"Pad 2 (warm)", 89},
	{"Pad 4 (choir)", 91},
	{"Crystal", 98},
	{"Atmosphere", 99},
	{"Brightness", 100},
	{"Gunshot", 127},
}

// ---------------------------------------------------------------------------
// Presets (equivalent to Syntrillium's named favorites)
// ---------------------------------------------------------------------------

type Preset struct {
	Name        string
	ScaleIdx    int
	InstrIdx    int
	BaseNote    int // MIDI note: 60=C4
	NumChimes   int
	WindSpeed   float64 // 0.0 (calm) – 1.0 (storm)
	Reverb      int     // 0-127
	Octaves     int     // how many octaves to span
}

var Presets = []Preset{
	{"Soft Ambiance", 0, 1, 60, 5, 0.15, 80, 2},         // pentatonic marimba
	{"Gentle Breeze", 0, 0, 60, 6, 0.2, 90, 2},           // pentatonic tubular bells
	{"Wind in the Trees", 2, 3, 55, 7, 0.35, 70, 3},      // major guitar
	{"Storm", 0, 0, 48, 10, 0.85, 60, 3},                 // pentatonic bells, fast
	{"Japanese Garden", 14, 16, 60, 5, 0.2, 100, 2},      // hirajoshi sitar
	{"Glorious Sunrise", 2, 2, 60, 8, 0.3, 80, 3},        // major vibraphone
	{"Cathedral", 6, 7, 48, 6, 0.25, 120, 3},             // lydian organ
	{"Blues Alley", 12, 8, 57, 6, 0.4, 64, 2},            // blues guitar
	{"Steel Drum Party", 0, 19, 60, 8, 0.6, 50, 2},       // pentatonic steel drums
	{"Gunfight!", 13, 28, 48, 12, 0.95, 20, 2},           // chromatic gunshots
	{"Twilight", 4, 23, 55, 5, 0.18, 110, 2},             // dorian warm pad
	{"Meditation", 14, 24, 60, 4, 0.1, 120, 2},           // hirajoshi choir pad
}

//----------------------------------------------------------------------------
// Pink Noise capability
//----------------------------------------------------------------------------

// PinkNoise generates pink (1/f) noise using the Voss-McCartney algorithm.
// Output values cluster around 0, roughly in the range [-1, 1].
type PinkNoise struct {
	rows [16]float64
	sum  float64
}

func (p *PinkNoise) Next() float64 {
	idx := rand.Intn(16)
	newVal := rand.Float64()*2 - 1
	p.sum += newVal - p.rows[idx]
	p.rows[idx] = newVal
	return p.sum / 16.0
}


// ---------------------------------------------------------------------------
// Wind physics simulation
// ---------------------------------------------------------------------------

// WindState models a pendulum being driven by wind. Each "chime" is a
// pendulum; when it swings past a neighbour it triggers a note, just like
// real wind chimes.
type Chime struct {
	angle    float64 // radians from rest
	velocity float64 // angular velocity
	noteIdx  int     // index into noteSet
	cooldown float64 // seconds before this chime can ring again
	channel  byte
}

const (
	damping    = 0.995  // energy loss per physics tick
	chimeGap   = 0.25   // radians: if |angle - neighbour.angle| < gap, ring
	cooldownSec = 0.4   // minimum gap between notes on same chime
)

type Engine struct {
	mu          sync.Mutex
	midi        *MIDIWriter
	chimes      []*Chime
	noteSet     []int
	windSpeed   float64 // 0.0–1.0
	scale       Scale
	instrument  Instrument
	baseNote    int
	numChimes   int
	reverb      int
	running     bool
	activeNotes map[byte]int // channel -> note (for note-off scheduling)
	pink        PinkNoise   // <-- add this
	smoothedGust float64    // <-- add this, smoothed pink output
}

func NewEngine(mw *MIDIWriter) *Engine {
	return &Engine{
		midi:        mw,
		windSpeed:   0.25,
		scale:       Scales[0],
		instrument:  Instruments[0],
		baseNote:    60,
		numChimes:   6,
		reverb:      80,
		activeNotes: make(map[byte]int),
	}
}

func (e *Engine) ApplyPreset(p Preset) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scale = Scales[p.ScaleIdx]
	e.instrument = Instruments[p.InstrIdx]
	e.baseNote = p.BaseNote
	e.numChimes = p.NumChimes
	e.windSpeed = p.WindSpeed
	e.reverb = p.Reverb
	e.rebuildChimes()
	e.applyMIDISetup()
}

func (e *Engine) rebuildChimes() {
	// stop existing notes
	for ch := byte(0); ch < byte(e.numChimes); ch++ {
		e.midi.AllNotesOff(ch)
	}

	e.noteSet = buildNoteSet(e.baseNote, e.scale, e.numChimes)
	if len(e.noteSet) == 0 {
		e.noteSet = []int{60, 62, 64, 67, 69}
	}

	e.chimes = make([]*Chime, e.numChimes)
	for i := 0; i < e.numChimes; i++ {
		// space chimes evenly and give them slightly different resting positions
		angle := (float64(i)/float64(e.numChimes))*0.3 - 0.15
		noteIdx := (i * len(e.noteSet) / e.numChimes) % len(e.noteSet)
		e.chimes[i] = &Chime{
			angle:   angle,
			noteIdx: noteIdx,
			channel: byte(i % 16),
		}
	}
}

func (e *Engine) applyMIDISetup() {
	seen := map[byte]bool{}
	for _, c := range e.chimes {
		if !seen[c.channel] {
			e.midi.ProgramChange(c.channel, e.instrument.PC)
			// Reverb (CC 91)
			e.midi.ControlChange(c.channel, 91, byte(e.reverb))
			// Expression (CC 11) full
			e.midi.ControlChange(c.channel, 11, 127)
			seen[c.channel] = true
		}
	}
}

func (e *Engine) Start() {
	e.mu.Lock()
	e.running = true
	e.rebuildChimes()
	e.applyMIDISetup()
	e.mu.Unlock()

	go e.physicsLoop()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	e.running = false
	for ch := byte(0); ch < 16; ch++ {
		e.midi.AllNotesOff(ch)
	}
	e.mu.Unlock()
}

// physicsLoop runs the wind-chime simulation.
// Physics tick: 20ms. Notes are triggered when chimes collide.

func (e *Engine) physicsLoop() {
	dt := 0.020
	ticker := time.NewTicker(time.Duration(dt*1000) * time.Millisecond)
	defer ticker.Stop()

	type pendingOff struct {
		at   time.Time
		ch   byte
		note byte
	}
	var offs []pendingOff

	for range ticker.C {
		e.mu.Lock()
		if !e.running {
			e.mu.Unlock()
			return
		}
		
		now := time.Now()
		
		// --- Pink noise gust modulation ---
		// Smooth the raw pink noise so gusts ramp in/out over ~1-2 seconds
		// rather than flickering every tick.
		raw := e.pink.Next()
		const smoothing = 0.02 // lower = slower, smoother gusts
		e.smoothedGust += (raw - e.smoothedGust) * smoothing

		// effectiveWind drifts around windSpeed by up to ±50%
		effectiveWind := e.windSpeed * (1.0 + e.smoothedGust*0.5)
		if effectiveWind < 0.02 {
			effectiveWind = 0.02
		}
		if effectiveWind > 1.0 {
			effectiveWind = 1.0
		}

		// Each chime has a chance of being struck this tick
		for _, c := range e.chimes {
			if c.cooldown > 0 {
				c.cooldown -= dt
				continue
			}

			strikeProb := effectiveWind * 0.15
			if rand.Float64() < strikeProb {
				minVel := int(effectiveWind * 60)
				randVel := int(rand.Float64() * effectiveWind * 60)
				vel := byte(minVel + randVel)
				if vel < 20 {
					vel = 20
				}
				if vel > 120 {
					vel = 120
				}
				
				noteNum := byte(e.noteSet[c.noteIdx%len(e.noteSet)])
				e.midi.NoteOn(c.channel, noteNum, vel)
				
				duration := time.Duration((1.5-effectiveWind)*800+200) * time.Millisecond
				offs = append(offs, pendingOff{
					at:   now.Add(duration),
					ch:   c.channel,
					note: noteNum,
				})
				
				c.cooldown = (1.0-effectiveWind)*0.8 + 0.1 + rand.Float64()*0.3
			}
		}
		
		e.mu.Unlock()
		
		// Process note offs
		remaining := offs[:0]
		for _, o := range offs {
			if now.After(o.at) {
				e.midi.NoteOff(o.ch, o.note)
			} else {
				remaining = append(remaining, o)
			}
		}
		offs = remaining
	}
}


// ---------------------------------------------------------------------------
// Settings mutators (safe to call from UI goroutine)
// ---------------------------------------------------------------------------

func (e *Engine) SetWindSpeed(s float64) {
	if s < 0.01 {
		s = 0.01
	}
	if s > 1.0 {
		s = 1.0
	}
	e.mu.Lock()
	e.windSpeed = s
	e.mu.Unlock()
}

func (e *Engine) SetScale(idx int) {
	e.mu.Lock()
	e.scale = Scales[idx]
	e.rebuildChimes()
	e.applyMIDISetup()
	e.mu.Unlock()
}

func (e *Engine) SetInstrument(idx int) {
	e.mu.Lock()
	e.instrument = Instruments[idx]
	e.applyMIDISetup()
	e.mu.Unlock()
}

func (e *Engine) SetBaseNote(n int) {
	if n < 21 {
		n = 21
	}
	if n > 84 {
		n = 84
	}
	e.mu.Lock()
	e.baseNote = n
	e.rebuildChimes()
	e.applyMIDISetup()
	e.mu.Unlock()
}

func (e *Engine) SetNumChimes(n int) {
	if n < 2 {
		n = 2
	}
	if n > 16 {
		n = 16
	}
	e.mu.Lock()
	e.numChimes = n
	e.rebuildChimes()
	e.applyMIDISetup()
	e.mu.Unlock()
}

func (e *Engine) GetSettings() (float64, string, string, int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.windSpeed, e.scale.Name, e.instrument.Name, e.baseNote, e.numChimes
}

// ---------------------------------------------------------------------------
// Note name helper
// ---------------------------------------------------------------------------

var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

func noteName(n int) string {
	return fmt.Sprintf("%s%d", noteNames[n%12], n/12-1)
}

// ---------------------------------------------------------------------------
// UI / interactive keyboard control
// ---------------------------------------------------------------------------

func windBar(speed float64) string {
	bars := int(speed * 30)
	s := "["
	for i := 0; i < 30; i++ {
		if i < bars {
			s += "="
		} else {
			s += " "
		}
	}
	label := "calm"
	switch {
	case speed > 0.8:
		label = "STORM"
	case speed > 0.6:
		label = "strong"
	case speed > 0.4:
		label = "breezy"
	case speed > 0.2:
		label = "gentle"
	}
	return fmt.Sprintf("%s] %s (%.0f%%)", s, label, speed*100)
}

func printHelp() {
	fmt.Fprintln(os.Stderr, `
╔═══════════════════════════════════════════════════════╗
║          Wind Chimes  ♪  Linux Edition                ║
║       Inspired by Syntrillium Wind Chimes 1.01        ║
╚═══════════════════════════════════════════════════════╝

 Controls (type a key then press Enter):
  +  /  -    Increase / decrease wind speed (10% steps)
  ++  / --   Large wind change (25% steps)
  n  /  p    Next / previous scale
  i          Next instrument
  I          Previous instrument
  b  /  B    Base note up / down one semitone
  c  /  C    More / fewer chimes
  r          Randomize all settings
  l          List presets
  1-9,0      Load preset (0 = preset 10)
  s          Show current settings
  ?          Show this help
  q          Quit
`)
}

func printSettings(e *Engine) {
	ws, sc, ins, bn, nc := e.GetSettings()
	fmt.Fprintf(os.Stderr, "\n  Wind speed : %s\n", windBar(ws))
	fmt.Fprintf(os.Stderr, "  Scale      : %s\n", sc)
	fmt.Fprintf(os.Stderr, "  Instrument : %s\n", ins)
	fmt.Fprintf(os.Stderr, "  Base note  : %s (%d)\n", noteName(bn), bn)
	fmt.Fprintf(os.Stderr, "  Chimes     : %d\n\n", nc)
}

func printPresets() {
	fmt.Fprintln(os.Stderr, "\n  Presets:")
	for i, p := range Presets {
		key := strconv.Itoa(i + 1)
		if i == 9 {
			key = "0"
		}
		fmt.Fprintf(os.Stderr, "  [%s] %-24s  wind=%.0f%%  scale=%s\n",
			key, p.Name, p.WindSpeed*100, Scales[p.ScaleIdx].Name)
	}
	fmt.Fprintln(os.Stderr, )
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	outFile := flag.String("out", "", "Write raw MIDI to this file/device (default: stdout)")
	presetFlag := flag.Int("preset", 1, "Start with preset number (1-12)")
	windFlag := flag.Float64("wind", -1, "Initial wind speed 0.0-1.0 (overrides preset)")
	listPresets := flag.Bool("list-presets", false, "List presets and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `windchimes - Linux reimplementation of Syntrillium Wind Chimes

Usage:
  windchimes [options] | timidity -
  windchimes [options] | fluidsynth -a alsa -m alsa-raw -
  windchimes [options] --out /dev/midi0

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, `
Examples:
  windchimes | timidity -
  windchimes --preset 5 --wind 0.6 | timidity -
  windchimes --out /dev/snd/midiC0D0
`)
	}
	flag.Parse()

	if *listPresets {
		for i, p := range Presets {
			fmt.Fprintf(os.Stderr, "%2d. %-24s  wind=%.0f%%  scale=%-24s  instrument=%s\n",
				i+1, p.Name, p.WindSpeed*100, Scales[p.ScaleIdx].Name, Instruments[p.InstrIdx].Name)
		}
		return
	}

	// Open output
	var outF *os.File
	if *outFile != "" {
		var err error
		outF, err = os.OpenFile(*outFile, os.O_WRONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot open MIDI output %s: %v\n", *outFile, err)
			os.Exit(1)
		}
		defer outF.Close()
	} else {
		outF = os.Stdout
		// Redirect stderr so status messages don't pollute the MIDI stream
		os.Stderr.WriteString("") // ensure stderr is open
	}

	mw := NewMIDIWriter(outF)
	eng := NewEngine(mw)

	// Apply initial preset
	pIdx := *presetFlag - 1
	if pIdx < 0 || pIdx >= len(Presets) {
		pIdx = 0
	}
	eng.ApplyPreset(Presets[pIdx])
	if *windFlag >= 0 {
		eng.SetWindSpeed(*windFlag)
	}

	eng.Start()

	// Print to stderr so it doesn't interfere with MIDI stdout
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  ♪  Wind Chimes running  ♪")
	fmt.Fprintf(os.Stderr, "     Preset: %s\n", Presets[pIdx].Name)
	fmt.Fprintln(os.Stderr, "     Type ? for help, q to quit")
	fmt.Fprintln(os.Stderr, "")
	printSettings(eng)

	// State for UI
	scaleIdx := Presets[pIdx].ScaleIdx
	instrIdx := Presets[pIdx].InstrIdx

	stdin := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "> ")
		line, _ := stdin.ReadString('\n')
		cmd := strings.TrimSpace(line)
		ws, _, _, bn, nc := eng.GetSettings()

		switch cmd {
		case "q", "quit", "exit":
			eng.Stop()
			fmt.Fprintln(os.Stderr, "Goodbye ♪")
			return

		case "+":
			eng.SetWindSpeed(ws + 0.1)
			ws, _, _, _, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Wind: %s\n", windBar(ws))

		case "++":
			eng.SetWindSpeed(ws + 0.25)
			ws, _, _, _, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Wind: %s\n", windBar(ws))

		case "-":
			eng.SetWindSpeed(ws - 0.1)
			ws, _, _, _, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Wind: %s\n", windBar(ws))

		case "--":
			eng.SetWindSpeed(ws - 0.25)
			ws, _, _, _, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Wind: %s\n", windBar(ws))

		case "n":
			scaleIdx = (scaleIdx + 1) % len(Scales)
			eng.SetScale(scaleIdx)
			fmt.Fprintf(os.Stderr, "  Scale: %s\n", Scales[scaleIdx].Name)

		case "p":
			scaleIdx = (scaleIdx - 1 + len(Scales)) % len(Scales)
			eng.SetScale(scaleIdx)
			fmt.Fprintf(os.Stderr, "  Scale: %s\n", Scales[scaleIdx].Name)

		case "i":
			instrIdx = (instrIdx + 1) % len(Instruments)
			eng.SetInstrument(instrIdx)
			fmt.Fprintf(os.Stderr, "  Instrument: %s\n", Instruments[instrIdx].Name)

		case "I":
			instrIdx = (instrIdx - 1 + len(Instruments)) % len(Instruments)
			eng.SetInstrument(instrIdx)
			fmt.Fprintf(os.Stderr, "  Instrument: %s\n", Instruments[instrIdx].Name)

		case "b":
			eng.SetBaseNote(bn + 1)
			_, _, _, bn, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Base note: %s\n", noteName(bn))

		case "B":
			eng.SetBaseNote(bn - 1)
			_, _, _, bn, _ = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Base note: %s\n", noteName(bn))

		case "c":
			eng.SetNumChimes(nc + 1)
			_, _, _, _, nc = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Chimes: %d\n", nc)

		case "C":
			eng.SetNumChimes(nc - 1)
			_, _, _, _, nc = eng.GetSettings()
			fmt.Fprintf(os.Stderr, "  Chimes: %d\n", nc)

		case "r":
			scaleIdx = rand.Intn(len(Scales))
			instrIdx = rand.Intn(len(Instruments) - 2) // skip gunshot in random
			eng.SetScale(scaleIdx)
			eng.SetInstrument(instrIdx)
			eng.SetBaseNote(48 + rand.Intn(24))
			eng.SetNumChimes(4 + rand.Intn(8))
			eng.SetWindSpeed(0.1 + rand.Float64()*0.7)
			fmt.Fprintln(os.Stderr, "  Settings randomized!")
			printSettings(eng)

		case "s":
			printSettings(eng)

		case "l":
			printPresets()

		case "?", "h", "help":
			printHelp()

		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
			n, _ := strconv.Atoi(cmd)
			if n == 0 {
				n = 10
			}
			n-- // 0-indexed
			if n < len(Presets) {
				eng.ApplyPreset(Presets[n])
				scaleIdx = Presets[n].ScaleIdx
				instrIdx = Presets[n].InstrIdx
				fmt.Fprintf(os.Stderr, "  Loaded preset: %s\n", Presets[n].Name)
				printSettings(eng)
			}

		case "":
			// just a blank Enter — do nothing

		default:
			fmt.Fprintf(os.Stderr, "  Unknown command %q — type ? for help\n", cmd)
		}
	}
}
