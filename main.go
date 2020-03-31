package main

func main() {
	var (
		v = settings()
		l = newLogger(v)
		g = newGracefulContext(l)

		a = newApp(
			WithLogger(l),
			WithConfig(v))
	)

	go a.Serve(g)
	go a.Worker(g)

	a.Wait()
}
