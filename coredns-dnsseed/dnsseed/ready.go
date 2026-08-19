package dnsseed

// Ready implements the ready.Readiness interface. Once this returns true,
// CoreDNS considers the plugin ready to serve queries.
func (ps LatticeSeeder) Ready() bool {
	return ps.seeder.Ready()
}
