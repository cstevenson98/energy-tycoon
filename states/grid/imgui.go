package gridstate

import (
	"fmt"
	"math"
	"sort"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/milo/pkg/ecs"
	"github.com/cstevenson98/milo/pkg/imgui"
)

func (s *GridState) renderNetworkPanel(w *imgui.WindowBuilder, net *network.ElectricalNetwork) {
	buses := net.Buses()
	branches := net.Branches()
	st := net.State

	s.renderSimulationPanel(w)
	w.Separator()

	s.renderSelectionPanel(w, net)
	w.Separator()

	w.Text("Topology")
	w.Text("  Buses: %d", len(buses))
	w.Text("  Branches: %d", len(branches))
	w.Text("  Dirty: %v", net.Dirty)
	w.Separator()

	w.Text("Load flow")
	if st == nil {
		w.Text("  (no state)")
		return
	}
	w.Text("  Converged: %v", st.Converged)
	w.Text("  Iterations: %d", st.Iterations)
	if st.LastError != "" {
		w.Text("  Error: %s", st.LastError)
	}
	w.Separator()

	var nGen, nLoad, nJunc int
	for _, b := range buses {
		switch b.Type {
		case network.BusGenerator:
			nGen++
		case network.BusLoad:
			nLoad++
		case network.BusJunction:
			nJunc++
		}
	}
	w.Text("Bus types")
	w.Text("  Generators: %d", nGen)
	w.Text("  Loads: %d", nLoad)
	w.Text("  Junctions: %d", nJunc)
	w.Separator()

	var pGen, pLoad, iMax float64
	for id, bs := range st.Buses {
		b, ok := buses[id]
		if !ok {
			continue
		}
		p := bs.Result.PInject
		if b.Type == network.BusGenerator || p > 0 {
			pGen += math.Max(p, 0)
		}
		if b.Type == network.BusLoad || p < 0 {
			pLoad += math.Max(-p, 0)
		}
	}
	for _, br := range st.Branches {
		if br.Result.CurrentMag > iMax {
			iMax = br.Result.CurrentMag
		}
	}
	w.Text("Power (solved)")
	w.Text("  Generation: %.2f kW", pGen/1000)
	w.Text("  Load: %.2f kW", pLoad/1000)
	w.Text("  Peak |I|: %.2f A", iMax)
	w.Separator()

	s.renderVoltageProfiles(w, net)
	w.Separator()

	s.renderBusHistoryCharts(w, net)
	w.Separator()

	w.TreeNode("Buses", func(w *imgui.WindowBuilder) {
		ids := make([]int, 0, len(buses))
		for id := range buses {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, raw := range ids {
			id := network.BusID(raw)
			b := buses[id]
			bs := st.Buses[id]
			if bs == nil {
				w.Text("bus %d (%s)", b.ID, b.Type)
				continue
			}
			angDeg := bs.Result.VoltAng * 180 / math.Pi
			w.Text("bus %d (%s)  %s", b.ID, b.Type, bs.Spec.Formulation.String())
			w.Text("  V=%.1f V ∠ %.2f°  P=%.2f kW  Q=%.2f kvar",
				bs.Result.VoltMag, angDeg, bs.Result.PInject/1000, bs.Result.QInject/1000)
		}
	})

	w.TreeNode("Branches", func(w *imgui.WindowBuilder) {
		ids := make([]int, 0, len(branches))
		for id := range branches {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, raw := range ids {
			id := network.BranchID(raw)
			br := branches[id]
			brs := st.Branches[id]
			rOhm := br.Resistance
			if brs == nil {
				w.Text("br %d: %d—%d  R=%.3f Ω", br.ID, br.From, br.To, rOhm)
				continue
			}
			w.Text("br %d: %d—%d  R=%.3f Ω  |I|=%.2f A  P=%.2f kW",
				br.ID, br.From, br.To, rOhm, brs.Result.CurrentMag, brs.Result.PFrom/1000)
		}
	})
}

const perBusPlotHeight = 220.0

// Voltage-profile plot presentation.
const voltageProfilePlotHeight = 440.0 // 2× per-bus history plots

// voltageProfileYPadOptions are selectable Y-axis half-widths as a fraction of
// NominalVoltageV (dropdown labels ↔ ±pad).
var voltageProfileYPadOptions = []struct {
	Label string
	Frac  float64
}{
	{"±1%", 0.01},
	{"±5%", 0.05},
	{"±10%", 0.10},
	{"±15%", 0.15},
	{"±20%", 0.20},
	{"±50%", 0.50},
}

// ohmToMetres converts cumulative branch resistance to metres assuming uniform
// LV cable (R ∝ length via grid.CableOhmPerKm).
func ohmToMetres(rOhm float64) float64 {
	if grid.CableOhmPerKm <= 0 {
		return 0
	}
	return rOhm * 1000 / grid.CableOhmPerKm
}

// renderVoltageProfiles draws one topological |V| vs distance plot per generator.
// Each first-hop branch out of the generator is its own LineXY series.
func (s *GridState) renderVoltageProfiles(w *imgui.WindowBuilder, net *network.ElectricalNetwork) {
	w.Text("Voltage profiles")
	if net == nil || net.State == nil {
		w.Text("  (no state)")
		return
	}

	labels := make([]string, len(voltageProfileYPadOptions))
	for i, opt := range voltageProfileYPadOptions {
		labels[i] = opt.Label
	}
	w.Combo("Y range", &s.voltageProfileYPadIdx, labels)
	pad := voltageProfileYPadOptions[0].Frac
	if idx := s.voltageProfileYPadIdx; idx >= 0 && idx < len(voltageProfileYPadOptions) {
		pad = voltageProfileYPadOptions[idx].Frac
	}
	yMin := network.NominalVoltageV * (1 - pad)
	yMax := network.NominalVoltageV * (1 + pad)

	ids := make([]int, 0)
	for id, b := range net.Buses() {
		if b.Type == network.BusGenerator {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)
	if len(ids) == 0 {
		w.Text("  (none)")
		return
	}

	nan := math.NaN()
	for _, raw := range ids {
		genID := network.BusID(raw)
		feeders := net.VoltageProfiles(genID)
		if len(feeders) == 0 {
			w.Text("Gen bus %d  (no feeders)", genID)
			continue
		}
		w.Text("Gen bus %d", genID)
		w.Plot(fmt.Sprintf("Vprofile%d", genID), voltageProfilePlotHeight, func(p *imgui.PlotBuilder) {
			p.SetupAxesYLimits("m from gen", "V", yMin, yMax)
			for _, fp := range feeders {
				if len(fp.Segments) == 0 {
					continue
				}
				xs := make([]float64, 0, len(fp.Segments)*3)
				ys := make([]float64, 0, len(fp.Segments)*3)
				for i, seg := range fp.Segments {
					if i > 0 {
						xs = append(xs, nan)
						ys = append(ys, nan)
					}
					xs = append(xs, ohmToMetres(seg.DistFrom), ohmToMetres(seg.DistTo))
					ys = append(ys, seg.VFrom, seg.VTo)
				}
				p.LineXY(fmt.Sprintf("feeder br%d", fp.RootBranch), xs, ys)
			}
		})
	}
}

// busHist holds one bus entity's solve history in kW / kvar / volts.
// hours is hour-of-day in [0, 24), with NaN breaks at midnight wraps.
type busHist struct {
	id        network.BusID
	hours     []float64
	pKW       []float64
	qKVAR     []float64
	vVolts    []float64
	timeLabel string // e.g. "14:05–16:20"
}

// hourOfDaySeries maps HoursSinceEpoch → [0, 24) and inserts NaN separators
// when the series crosses midnight so the line does not draw backwards.
func hourOfDaySeries(hoursSinceEpoch, ys []float64) (xs, out []float64) {
	if len(hoursSinceEpoch) == 0 || len(ys) == 0 {
		return nil, nil
	}
	n := len(hoursSinceEpoch)
	if len(ys) < n {
		n = len(ys)
	}
	xs = make([]float64, 0, n+n/4)
	out = make([]float64, 0, n+n/4)
	var prev float64
	for i := 0; i < n; i++ {
		h := hoursSinceEpoch[i]
		tod := h - 24*math.Floor(h/24)
		if i > 0 && tod < prev {
			xs = append(xs, math.NaN())
			out = append(out, math.NaN())
		}
		xs = append(xs, tod)
		out = append(out, ys[i])
		prev = tod
	}
	return xs, out
}

func historyTimeLabel(hoursSinceEpoch []float64) string {
	if len(hoursSinceEpoch) == 0 {
		return ""
	}
	start := sim.FormatClockHM(sim.MsFromHoursSinceEpoch(hoursSinceEpoch[0]))
	end := sim.FormatClockHM(sim.MsFromHoursSinceEpoch(hoursSinceEpoch[len(hoursSinceEpoch)-1]))
	if start == end {
		return start
	}
	return start + "–" + end
}

// collectBusHistories returns BusHistory series for buses of the given type,
// sorted by bus ID. If consumerSign is true, P/Q are negated (load demand);
// otherwise they stay in generator-convention kW (positive = injection).
func (s *GridState) collectBusHistories(
	net *network.ElectricalNetwork,
	typ network.BusType,
	consumerSign bool,
) []busHist {
	busHistMap := ecs.NewMap1[network.BusHistory](s.World())
	ids := make([]int, 0)
	for id, b := range net.Buses() {
		if b.Type != typ {
			continue
		}
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	out := make([]busHist, 0, len(ids))
	for _, raw := range ids {
		id := network.BusID(raw)
		b := net.Buses()[id]
		h := busHistMap.Get(b.Entity)
		if h == nil || h.P.Len() == 0 {
			continue
		}
		absHours := h.T.Values()
		p := h.P.Values()
		q := h.Q.Values()
		v := h.V.Values()
		pKW := make([]float64, len(p))
		qKVAR := make([]float64, len(q))
		sign := 1.0
		if consumerSign {
			sign = -1.0
		}
		for i := range p {
			pKW[i] = sign * p[i] / 1000
			if i < len(q) {
				qKVAR[i] = sign * q[i] / 1000
			}
		}
		hx, pPlot := hourOfDaySeries(absHours, pKW)
		_, qPlot := hourOfDaySeries(absHours, qKVAR)
		_, vPlot := hourOfDaySeries(absHours, v)
		out = append(out, busHist{
			id:        id,
			hours:     hx,
			pKW:       pPlot,
			qKVAR:     qPlot,
			vVolts:    vPlot,
			timeLabel: historyTimeLabel(absHours),
		})
	}
	return out
}

func (s *GridState) renderBusHistoryCharts(w *imgui.WindowBuilder, net *network.ElectricalNetwork) {
	gens := s.collectBusHistories(net, network.BusGenerator, false)
	houses := s.collectBusHistories(net, network.BusLoad, true)

	w.Text("Generators")
	if len(gens) == 0 {
		w.Text("  (none with history yet)")
	} else {
		for _, h := range gens {
			s.renderOneBusHistory(w, "Gen", h, "kW / kvar (+gen)")
		}
	}
	w.Separator()

	w.Text("Houses")
	if len(houses) == 0 {
		w.Text("  (none with history yet)")
	} else {
		for _, h := range houses {
			s.renderOneBusHistory(w, "House", h, "kW / kvar (demand)")
		}
	}
}

// renderOneBusHistory draws one bus's P/Q (left) and |V| (right) plots.
func (s *GridState) renderOneBusHistory(
	w *imgui.WindowBuilder,
	kind string,
	h busHist,
	pqAxis string,
) {
	if h.timeLabel != "" {
		w.Text("%s bus %d  %s  (%d samples)", kind, h.id, h.timeLabel, len(h.pKW))
	} else {
		w.Text("%s bus %d  (%d samples)", kind, h.id, len(h.pKW))
	}
	w.Columns(2)
	w.Plot(fmt.Sprintf("%s%d P/Q", kind, h.id), perBusPlotHeight, func(p *imgui.PlotBuilder) {
		p.SetupAxesXLimits("hour (24h)", pqAxis, 0, 24)
		p.LineXY("P", h.hours, h.pKW)
		p.LineXY("Q", h.hours, h.qKVAR)
	})
	w.NextColumn()
	w.Plot(fmt.Sprintf("%s%d |V|", kind, h.id), perBusPlotHeight, func(p *imgui.PlotBuilder) {
		p.SetupAxesXLimits("hour (24h)", "V", 0, 24)
		p.LineXY("|V|", h.hours, h.vVolts)
	})
	w.Columns(1)
}

// renderSelectionPanel shows metadata for the currently selected grid cell.
func (s *GridState) renderSimulationPanel(w *imgui.WindowBuilder) {
	clock := ecs.GetResource[sim.SimClock](s.World())
	if clock == nil {
		w.Text("Simulation")
		w.Text("  (no clock)")
		return
	}

	w.Text("Simulation")
	w.Text("  Time: %s", sim.FormatSimTime(clock.NowMs))
	if clock.Playing {
		w.Text("  Status: Playing")
	} else {
		w.Text("  Status: Paused")
	}

	w.Button("Play", func() { clock.Playing = true })
	w.SameLine()
	w.Button("Pause", func() { clock.Playing = false })

	w.Text("  Speed")
	for i := 0; i < sim.NumSpeeds; i++ {
		idx := i
		label := sim.SpeedLabels[idx]
		if clock.SpeedIndex == idx {
			label = "[" + label + "]"
		}
		if i > 0 {
			w.SameLine()
		}
		w.Button(label, func() { clock.SetSpeedIndex(idx) })
	}

	s.renderAmbientPanel(w, clock)
	s.renderApplianceSummary(w)
}

func (s *GridState) renderAmbientPanel(w *imgui.WindowBuilder, clock *sim.SimClock) {
	amb := ecs.GetResource[appliance.AmbientTemp](s.World())
	if amb == nil {
		return
	}
	w.Separator()
	w.Text("Weather")
	base := appliance.DiurnalBaseC(sim.DayFraction(clock.NowMs))
	w.Text("  Outdoor: %.1f °C  (diurnal %.1f °C)", amb.OutdoorC, base)
	w.Text("  Day cycle: %.0f ± %.0f °C  + noise",
		appliance.OutdoorMeanC, appliance.OutdoorAmplitudeC)
	w.Text("  HVAC setpoints: N(%.0f, %.0f²) °C  deadband ±%.0f",
		appliance.HVACSetpointMeanC, appliance.HVACSetpointSigmaC, appliance.HVACDeadbandC)
}

func (s *GridState) renderApplianceSummary(w *imgui.WindowBuilder) {
	houses := ecs.NewFilter2[grid.HouseLoad, appliance.HouseAppliances](s.World())
	n := 0
	onCount := 0
	var pSum, qSum float64
	houses.Each(func(_ ecs.Entity, hl *grid.HouseLoad, ha *appliance.HouseAppliances) {
		if hl.Source != grid.DemandAppliances {
			return
		}
		n++
		pSum += hl.PKw
		qSum += hl.QKw
		for i := range ha.Items {
			if ha.Items[i].On {
				onCount++
			}
		}
	})
	if n == 0 {
		return
	}
	w.Separator()
	w.Text("Appliance loads")
	w.Text("  Houses: %d  devices on: %d", n, onCount)
	w.Text("  Total demand: %.2f kW  %.2f kvar", pSum, qSum)
}

func (s *GridState) renderApplianceLine(w *imgui.WindowBuilder, inst *appliance.Instance) {
	on := "off"
	if inst.On {
		on = "on"
	}
	liveP, liveQ := 0.0, 0.0
	if b := appliance.Lookup(inst.Kind); b != nil {
		liveP, liveQ = b.PowerKW(inst)
	}

	switch inst.Kind {
	case appliance.KindHVAC:
		mode := "idle"
		indoor := appliance.IndoorC(inst)
		set := appliance.SetpointC(inst)
		if inst.On {
			if indoor > set {
				mode = "cool"
			} else {
				mode = "heat"
			}
		}
		w.Text("  %s: %s (%s)", inst.Kind, on, mode)
		w.Text("    indoor %.1f °C  set %.1f °C", indoor, set)
		w.Text("    draw %.2f kW / %.2f kvar  rated %.2f kW", liveP, liveQ, inst.RatedPKw)
	case appliance.KindFridge:
		w.Text("  %s: %s", inst.Kind, on)
		w.Text("    draw %.2f kW / %.2f kvar  rated %.2f kW", liveP, liveQ, inst.RatedPKw)
		w.Text("    cycle in %s", appliance.FormatDuration(appliance.FridgeTimerMs(inst)))
	default:
		w.Text("  %s: %s", inst.Kind, on)
		w.Text("    draw %.2f kW / %.2f kvar  rated %.2f kW", liveP, liveQ, inst.RatedPKw)
	}
}

func (s *GridState) renderSelectionPanel(w *imgui.WindowBuilder, net *network.ElectricalNetwork) {
	w.Text("Selection")
	placement := ecs.GetResource[grid.PlacementState](s.World())
	if placement == nil || !placement.HasSelection {
		w.Text("  No cell selected (clear tool with C, then click)")
		return
	}

	cell := placement.SelectedCell
	w.Text("  Cell: (%d, %d)", cell.Col, cell.Row)

	occupancy := ecs.GetResource[grid.GridOccupancy](s.World())
	if occupancy == nil {
		return
	}
	e, ok := occupancy.Cells[cell]
	if !ok {
		w.Text("  Occupant: empty")
		return
	}

	kind := "unknown"
	if go_ := ecs.NewMap1[grid.GridObject](s.World()).Get(e); go_ != nil {
		kind = go_.Kind.KindLabel()
	}
	w.Text("  Occupant: %s", kind)

	if hl := ecs.NewMap1[grid.HouseLoad](s.World()).Get(e); hl != nil {
		switch hl.Source {
		case grid.DemandAppliances:
			w.Text("  Demand: appliances")
		default:
			prof := grid.LookupProfile(hl.Profile)
			w.Text("  Demand: %s  peak=%.2f kW", prof.Name, hl.PeakKW)
		}
		w.Text("    P=%.2f kW  Q=%.2f kvar", hl.PKw, hl.QKw)
	}
	if ha := ecs.NewMap1[appliance.HouseAppliances](s.World()).Get(e); ha != nil {
		w.Text("  Appliances")
		for i := range ha.Items {
			s.renderApplianceLine(w, &ha.Items[i])
		}
	}
	if gp := ecs.NewMap1[grid.GeneratorProps](s.World()).Get(e); gp != nil {
		w.Text("  MaxOutput: %.1f kW", gp.MaxOutputKW)
	}
	if lsp := ecs.NewMap1[grid.LineSegmentProps](s.World()).Get(e); lsp != nil {
		w.Text("  R=%.4f Ω  X=%.4f Ω", lsp.ResistanceOhm, lsp.ReactanceOhm)
	}
	if lp := ecs.NewMap1[grid.LinePath](s.World()).Get(e); lp != nil {
		hops := len(lp.Cells) - 1
		if hops < 1 {
			hops = 1
		}
		w.Text("  Path: %d cells (%.0f m)", len(lp.Cells), float64(hops)*grid.CellLengthM)
		if ep := ecs.NewMap1[grid.LineEndpoints](s.World()).Get(e); ep != nil && ep.Wired {
			w.Text("  Branch %d  buses %d–%d", ep.BranchID, ep.FromBus, ep.ToBus)
		}
		return // lines have no NetworkLink / bus
	}

	link := ecs.NewMap1[network.NetworkLink](s.World()).Get(e)
	if link == nil || net == nil || net.State == nil {
		return
	}
	bus, ok := net.Bus(link.BusID)
	if !ok {
		return
	}
	bs := net.State.Buses[link.BusID]
	if bs == nil {
		w.Text("  Bus %d (%s)", bus.ID, bus.Type)
		return
	}
	w.Text("  Bus %d (%s)  %s", bus.ID, bus.Type, bs.Spec.Formulation.String())
	angDeg := bs.Result.VoltAng * 180 / math.Pi
	w.Text("  V=%.1f V ∠ %.2f°", bs.Result.VoltMag, angDeg)
	w.Text("  P=%.2f kW  Q=%.2f kvar", bs.Result.PInject/1000, bs.Result.QInject/1000)
	if h := ecs.NewMap1[network.BusHistory](s.World()).Get(e); h != nil {
		w.Text("  History samples: %d / %d", h.V.Len(), h.V.Cap())
	}
}
