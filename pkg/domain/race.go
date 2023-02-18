package domain

type Race struct {
	ID   string
	Laps []Lap // sorted lap list
}
