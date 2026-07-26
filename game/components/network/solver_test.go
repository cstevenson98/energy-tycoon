package network_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
)

// helper: spawn a dummy ECS entity so we can attach a bus to it.
func newEntity(w *ecs.World) ecs.Entity {
	return ecs.NewMap1[struct{}](w).NewEntity(&struct{}{})
}

// helper: build a fresh network + world, return them.
func emptyNet() (*network.ElectricalNetwork, *ecs.World) {
	return network.NewElectricalNetwork(), ecs.NewWorld()
}

func mustAddBus(t *testing.T, n *network.ElectricalNetwork, e ecs.Entity, typ network.BusType) *network.Bus {
	t.Helper()
	b, err := n.AddBus(e, typ)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	return b
}

// TestFlatStart: a network with only a slack bus should converge immediately
// with no mismatch.
func TestFlatStart(t *testing.T) {
	net, w := emptyNet()
	e0 := newEntity(w)
	bus0 := mustAddBus(t, net, e0, network.BusGenerator)
	net.SetBusSpec(bus0.ID, network.SlackSpec(1.0, 0.0))

	s := network.NewLoadflowSolver()
	if err := s.Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if !net.State.Converged {
		t.Fatal("expected convergence for trivial slack-only network")
	}

	bs, _ := net.BusStateFor(bus0.ID)
	if math.Abs(bs.Result.VoltMag-1.0) > 1e-9 {
		t.Errorf("|V| = %v, want 1.0", bs.Result.VoltMag)
	}
}

// TestTwoBusPureResistive verifies the NR solver against the analytical
// solution of a 2-bus network with one slack bus and one PQ load bus,
// connected by a purely resistive branch R = 1 pu.
//
// Network:
//
//	Bus 0 (Slack, V=1∠0°) ── R=1 ── Bus 1 (PQ, P_spec=−0.05, Q_spec=0)
//
// Generator convention: P_spec < 0 = absorbing from the network.
//
// Analytical solution (from the DC power equation for θ=0, B=0):
//
//	P_1 = V_1·(V_1 − 1) = −0.05
//	V_1² − V_1 + 0.05 = 0
//	V_1 = (1 + √0.8) / 2  ≈  0.9472    (high-voltage root)
//	δ_1 = 0   (no reactive power → no angle, from Q_1 = 0)
func TestTwoBusPureResistive(t *testing.T) {
	const R = 1.0
	const Pspec = -0.05
	wantV := (1.0 + math.Sqrt(0.8)) / 2.0 // ≈ 0.9472

	net, w := emptyNet()
	e0 := newEntity(w)
	e1 := newEntity(w)

	bus0 := mustAddBus(t, net, e0, network.BusGenerator)
	net.SetBusSpec(bus0.ID, network.SlackSpec(1.0, 0.0))

	bus1 := mustAddBus(t, net, e1, network.BusLoad)
	net.SetBusSpec(bus1.ID, network.PQSpec(Pspec, 0.0))

	net.AddBranch(bus0.ID, bus1.ID, R, 0)

	s := network.NewLoadflowSolver()
	if err := s.Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if !net.State.Converged {
		t.Fatalf("did not converge, iterations=%d", net.State.Iterations)
	}

	bs1, _ := net.BusStateFor(bus1.ID)
	if math.Abs(bs1.Result.VoltMag-wantV) > 1e-6 {
		t.Errorf("|V_1| = %.8f, want %.8f (err=%.2e)", bs1.Result.VoltMag, wantV, math.Abs(bs1.Result.VoltMag-wantV))
	}
	if math.Abs(bs1.Result.VoltAng) > 1e-6 {
		t.Errorf("δ_1 = %.2e, want 0", bs1.Result.VoltAng)
	}
	t.Logf("converged in %d iterations: |V_1|=%.8f δ_1=%.4f°",
		net.State.Iterations, bs1.Result.VoltMag, bs1.Result.VoltAng*180/math.Pi)

	// Verify P_calc at bus 1 matches P_spec.
	if math.Abs(bs1.Result.PInject-Pspec) > 1e-6 {
		t.Errorf("P_calc_1 = %.8f, want %.8f", bs1.Result.PInject, Pspec)
	}
}

// TestThreeBusResistive checks a simple three-bus radial network:
//
//	Bus 0 (Slack, V=1∠0°) ── R=0.1 ── Bus 1 (PQ, P=−0.02, Q=0) ── R=0.1 ── Bus 2 (PQ, P=−0.02, Q=0)
//
// We check that:
//  1. The solver converges.
//  2. V_2 < V_1 < V_0 (voltage decreases along the feeder).
//  3. P_calc matches P_spec at buses 1 and 2 to within tolerance.
func TestThreeBusResistive(t *testing.T) {
	const R = 0.1
	const Pload = -0.02

	net, w := emptyNet()
	e0, e1, e2 := newEntity(w), newEntity(w), newEntity(w)

	b0 := mustAddBus(t, net, e0, network.BusGenerator)
	net.SetBusSpec(b0.ID, network.SlackSpec(1.0, 0.0))

	b1 := mustAddBus(t, net, e1, network.BusLoad)
	net.SetBusSpec(b1.ID, network.PQSpec(Pload, 0.0))

	b2 := mustAddBus(t, net, e2, network.BusLoad)
	net.SetBusSpec(b2.ID, network.PQSpec(Pload, 0.0))

	net.AddBranch(b0.ID, b1.ID, R, 0)
	net.AddBranch(b1.ID, b2.ID, R, 0)

	s := network.NewLoadflowSolver()
	if err := s.Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if !net.State.Converged {
		t.Fatalf("did not converge, iterations=%d", net.State.Iterations)
	}

	bs0, _ := net.BusStateFor(b0.ID)
	bs1, _ := net.BusStateFor(b1.ID)
	bs2, _ := net.BusStateFor(b2.ID)

	t.Logf("converged in %d iters: V0=%.6f V1=%.6f V2=%.6f",
		net.State.Iterations, bs0.Result.VoltMag, bs1.Result.VoltMag, bs2.Result.VoltMag)

	if bs1.Result.VoltMag >= bs0.Result.VoltMag {
		t.Errorf("expected V_1 < V_0, got V_1=%.6f V_0=%.6f", bs1.Result.VoltMag, bs0.Result.VoltMag)
	}
	if bs2.Result.VoltMag >= bs1.Result.VoltMag {
		t.Errorf("expected V_2 < V_1, got V_2=%.6f V_1=%.6f", bs2.Result.VoltMag, bs1.Result.VoltMag)
	}

	const tol = 1e-5
	for _, tc := range []struct {
		name  string
		bs    *network.BusState
		pSpec float64
	}{
		{"bus1", bs1, Pload},
		{"bus2", bs2, Pload},
	} {
		if math.Abs(tc.bs.Result.PInject-tc.pSpec) > tol {
			t.Errorf("%s: P_calc=%.8f, want %.8f", tc.name, tc.bs.Result.PInject, tc.pSpec)
		}
	}
}

// TestNoSlackBusReturnsError ensures Solve returns an error when no slack bus
// is present (the system would be singular).
func TestNoSlackBusReturnsError(t *testing.T) {
	net, w := emptyNet()
	e := newEntity(w)
	b := mustAddBus(t, net, e, network.BusLoad)
	net.SetBusSpec(b.ID, network.PQSpec(-0.1, 0))

	s := network.NewLoadflowSolver()
	if err := s.Solve(net); err == nil {
		t.Fatal("expected error for network with no slack bus")
	}
}

// lineCellR is one 10 m cell of ≈185 mm² Al LV feeder (0.164 Ω/km).
const lineCellR = 0.00164

// TestLVFeeder mirrors the in-game LV setup: a 230V slack, two line cells
// (~20 m), and a house load of 15 kW + 5 kVAR.
func TestLVFeeder(t *testing.T) {
	const PloadW = -15000.0
	const QloadVAR = -5000.0

	net, w := emptyNet()
	e0, e1, e2 := newEntity(w), newEntity(w), newEntity(w)

	b0 := mustAddBus(t, net, e0, network.BusGenerator) // default SlackSpec(230, 0)
	b1 := mustAddBus(t, net, e1, network.BusJunction)
	b2 := mustAddBus(t, net, e2, network.BusLoad)
	net.SetBusSpec(b2.ID, network.PQSpec(PloadW, QloadVAR))

	net.AddBranch(b0.ID, b1.ID, lineCellR, 0)
	net.AddBranch(b1.ID, b2.ID, lineCellR, 0)

	if err := network.NewLoadflowSolver().Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if !net.State.Converged {
		t.Fatalf("did not converge, iterations=%d", net.State.Iterations)
	}

	bs0, _ := net.BusStateFor(b0.ID)
	bs1, _ := net.BusStateFor(b1.ID)
	bs2, _ := net.BusStateFor(b2.ID)

	if math.Abs(bs0.Result.VoltMag-network.NominalVoltageV) > 1e-6 {
		t.Errorf("slack |V| = %.4f, want %.1f", bs0.Result.VoltMag, network.NominalVoltageV)
	}
	if bs1.Result.VoltMag >= bs0.Result.VoltMag {
		t.Errorf("expected V_1 < V_0, got V_1=%.2f V_0=%.2f", bs1.Result.VoltMag, bs0.Result.VoltMag)
	}
	if bs2.Result.VoltMag >= bs1.Result.VoltMag {
		t.Errorf("expected V_2 < V_1, got V_2=%.2f V_1=%.2f", bs2.Result.VoltMag, bs1.Result.VoltMag)
	}
	if math.Abs(bs2.Result.PInject-PloadW) > 1e-3 {
		t.Errorf("P_calc_2 = %.4f, want %.4f", bs2.Result.PInject, PloadW)
	}
	if math.Abs(bs2.Result.QInject-QloadVAR) > 1e-3 {
		t.Errorf("Q_calc_2 = %.4f, want %.4f", bs2.Result.QInject, QloadVAR)
	}

	// ~15 kW over ~20 m of 185 mm² Al: drop should be well under a volt.
	drop := bs0.Result.VoltMag - bs2.Result.VoltMag
	if drop <= 0 || drop > 1.0 {
		t.Errorf("unexpected feeder drop %.3f V (want (0, 1] V)", drop)
	}
	t.Logf("LV feeder: V0=%.2fV V1=%.2fV V2=%.2fV drop=%.3fV iters=%d",
		bs0.Result.VoltMag, bs1.Result.VoltMag, bs2.Result.VoltMag, drop, net.State.Iterations)
}

// TestLVFeederWithX checks that non-zero series reactance produces angle
// separation and non-zero reactive branch flow.
func TestLVFeederWithX(t *testing.T) {
	const PloadW = -15000.0
	const QloadVAR = -5000.0
	const x = lineCellR * 0.5 // X/R = 0.5

	net, w := emptyNet()
	e0, e1, e2 := newEntity(w), newEntity(w), newEntity(w)
	b0 := mustAddBus(t, net, e0, network.BusGenerator)
	b1 := mustAddBus(t, net, e1, network.BusJunction)
	b2 := mustAddBus(t, net, e2, network.BusLoad)
	net.SetBusSpec(b2.ID, network.PQSpec(PloadW, QloadVAR))
	net.AddBranch(b0.ID, b1.ID, lineCellR, x)
	net.AddBranch(b1.ID, b2.ID, lineCellR, x)

	if err := network.NewLoadflowSolver().Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	bs2, _ := net.BusStateFor(b2.ID)
	if math.Abs(bs2.Result.VoltAng) < 1e-6 {
		t.Fatalf("expected non-zero load angle with X>0, got %.6f", bs2.Result.VoltAng)
	}
	var qFlow float64
	for _, br := range net.State.Branches {
		qFlow += math.Abs(br.Result.QFrom)
	}
	if qFlow < 1e-3 {
		t.Fatalf("expected non-zero |Q| branch flow with X>0, got %.6f", qFlow)
	}
}

// TestLVHundredBusRadial is a ~100-bus radial feeder (slack + 98 junctions +
// one end load) at 10 m/cell with distribution-cable R. Confirms the tuned
// parameters stay within a healthy LV band for a kilometre-scale feeder.
func TestLVHundredBusRadial(t *testing.T) {
	const nJunction = 98
	const PloadW = -15000.0

	net, w := emptyNet()
	slackE := newEntity(w)
	slack := mustAddBus(t, net, slackE, network.BusGenerator)
	prev := slack.ID
	for i := 0; i < nJunction; i++ {
		e := newEntity(w)
		b := mustAddBus(t, net, e, network.BusJunction)
		net.AddBranch(prev, b.ID, lineCellR, 0)
		prev = b.ID
	}
	loadE := newEntity(w)
	load := mustAddBus(t, net, loadE, network.BusLoad)
	net.SetBusSpec(load.ID, network.PQSpec(PloadW, 0))
	net.AddBranch(prev, load.ID, lineCellR, 0)

	if err := network.NewLoadflowSolver().Solve(net); err != nil {
		t.Fatalf("solve error: %v", err)
	}
	if !net.State.Converged {
		t.Fatalf("did not converge, iterations=%d", net.State.Iterations)
	}

	bs0, _ := net.BusStateFor(slack.ID)
	bsL, _ := net.BusStateFor(load.ID)
	drop := bs0.Result.VoltMag - bsL.Result.VoltMag
	// ~1 km of 185 mm² Al at 15 kW: expect roughly 5–15 V drop, still >216 V.
	if bsL.Result.VoltMag < 216 {
		t.Errorf("end |V|=%.2f, want ≥216 V (LV −6%% band)", bsL.Result.VoltMag)
	}
	if drop < 3 || drop > 25 {
		t.Errorf("feeder drop %.2f V, want roughly 3–25 V for 1 km @ 15 kW", drop)
	}
	t.Logf("100-bus radial: buses=%d V0=%.2f Vend=%.2f drop=%.2fV iters=%d",
		len(net.Buses()), bs0.Result.VoltMag, bsL.Result.VoltMag, drop, net.State.Iterations)
}
