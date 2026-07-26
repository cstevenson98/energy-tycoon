package network

import "sort"

// ProfileSegment is one branch on a generator-rooted voltage profile:
// |V| at the near and far buses versus cumulative resistance from the generator.
type ProfileSegment struct {
	DistFrom, DistTo float64 // Ω from generator at near / far bus
	VFrom, VTo       float64 // |V| at near / far bus (volts)
}

// FeederProfile is the spanning-tree walk reachable through one branch that
// leaves a generator. All Segments share one plot series / colour.
type FeederProfile struct {
	RootBranch BranchID
	Segments   []ProfileSegment
}

// VoltageProfiles walks outward from generator bus gen, producing one feeder
// profile per incident branch. Distance is cumulative branch resistance (Ω).
// Loops are handled with a shared visited set (spanning forest); each branch
// appears in at most one segment across all feeders for this generator.
//
// Returns nil if gen is unknown or not a BusGenerator.
func (n *ElectricalNetwork) VoltageProfiles(gen BusID) []FeederProfile {
	b, ok := n.buses[gen]
	if !ok || b.Type != BusGenerator || n.State == nil {
		return nil
	}

	volt := func(id BusID) (float64, bool) {
		bs := n.State.Buses[id]
		if bs == nil {
			return 0, false
		}
		return bs.Result.VoltMag, true
	}

	rootIDs := append([]BranchID(nil), n.incidence[gen]...)
	sort.Slice(rootIDs, func(i, j int) bool { return rootIDs[i] < rootIDs[j] })

	visited := map[BusID]bool{gen: true}
	feeders := make([]FeederProfile, 0, len(rootIDs))

	type queueItem struct {
		id   BusID
		dist float64
	}

	for _, rootID := range rootIDs {
		br := n.branches[rootID]
		if br == nil {
			continue
		}
		far := br.To
		if far == gen {
			far = br.From
		}
		if visited[far] {
			continue
		}
		vGen, okGen := volt(gen)
		vFar, okFar := volt(far)
		if !okGen || !okFar {
			continue
		}

		fp := FeederProfile{RootBranch: rootID}
		distFar := br.Resistance
		fp.Segments = append(fp.Segments, ProfileSegment{
			DistFrom: 0,
			DistTo:   distFar,
			VFrom:    vGen,
			VTo:      vFar,
		})
		visited[far] = true

		queue := []queueItem{{id: far, dist: distFar}}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]

			inc := append([]BranchID(nil), n.incidence[cur.id]...)
			sort.Slice(inc, func(i, j int) bool { return inc[i] < inc[j] })

			for _, brID := range inc {
				nbr := n.branches[brID]
				if nbr == nil {
					continue
				}
				other := nbr.To
				if other == cur.id {
					other = nbr.From
				}
				if visited[other] {
					continue
				}
				vCur, okCur := volt(cur.id)
				vOther, okOther := volt(other)
				if !okCur || !okOther {
					continue
				}
				distOther := cur.dist + nbr.Resistance
				fp.Segments = append(fp.Segments, ProfileSegment{
					DistFrom: cur.dist,
					DistTo:   distOther,
					VFrom:    vCur,
					VTo:      vOther,
				})
				visited[other] = true
				queue = append(queue, queueItem{id: other, dist: distOther})
			}
		}

		feeders = append(feeders, fp)
	}

	return feeders
}
