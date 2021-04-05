package connections

import "math/rand"

// https://www.keithschwarz.com/darts-dice-coins/
type Sampler struct {
	randomGenerator *rand.Rand
	probabilities   []float64
	alias           []int
}

type workList []int

func (wl *workList) push(e int) {
	*wl = append(*wl, e)
}

func (wl *workList) pop() int {
	l := len(*wl) - 1
	n := (*wl)[l]
	*wl = (*wl)[:l]
	return n
}

func NewSampler(probabilities []float64, source rand.Source) *Sampler {
	sampler := &Sampler{}
	var (
		small workList
		large workList
	)
	n := len(probabilities)
	sampler.randomGenerator = rand.New(source)
	sampler.probabilities = make([]float64, n)
	sampler.alias = make([]int, n)
	// Compute scaled probabilities.
	p := make([]float64, n)
	for i := 0; i < n; i++ {
		p[i] = probabilities[i] * float64(n)
	}
	for i, pi := range p {
		if pi < 1 {
			small = append(small, i)
		} else {
			large = append(large, i)
		}
	}
	for len(large) > 0 && len(small) > 0 {
		l, g := small.pop(), large.pop()
		sampler.probabilities[l] = p[l]
		sampler.alias[l] = g
		p[g] = (p[g] + p[l]) - 1
		if p[g] < 1 {
			small.push(g)
		} else {
			large.push(g)
		}
	}
	for len(large) > 0 {
		g := large.pop()
		sampler.probabilities[g] = 1
	}
	for len(small) > 0 {
		l := small.pop()
		sampler.probabilities[l] = 1
	}
	return sampler
}

func (g *Sampler) Next() int {
	n := len(g.alias)
	i := g.randomGenerator.Intn(n)
	if g.randomGenerator.Float64() < g.probabilities[i] {
		return i
	}
	return g.alias[i]
}
