// packaage uniqer reduce get may laps from chan end send redused lap to chan

package uniqer

import (
	"chicha/Packages/rfid/models"
	"time"
)

func Uniq(in <-chan models.Lap) <-chan models.AverageLap {
	out := make(chan models.AverageLap)

	go serve(in, out)

	return out
}

var racers = map[string]chan<- models.Lap{}

func serve(in <-chan models.Lap, out chan<- models.AverageLap) {
	for l := range in { // for wait for lap, break if chan closed
		r, ok := racers[l.TagID]
		if ok { // if created, add to lap
			// check timeout
			r <- l
			continue
		}

		// if not exist add new racer
		r = newRacer(out)
		r <- l // wait until gorutine run
		racers[l.TagID] = r
	}
}

func newRacer(out chan<- models.AverageLap) chan<- models.Lap {
	in := make(chan models.Lap)
	go run(in, out)
	return in
}

// todo:
// remove duplicates:
// first time, average time
// filter by tagID, antenna№, time 1s
func run(in <-chan models.Lap, filtered chan<- models.AverageLap) {
	var laps []models.Lap
	var timer <-chan time.Time // mb jitter lower or bigger second

	for {
		select {

		case l := <-in:
			if len(laps) == 0 {
				timer = time.After(time.Second) // init timer
			}
			laps = append(laps, l)

		case <-timer:
			firstLap := laps[0]
			averageLap := models.AverageLap{
				TagID:             firstLap.TagID,
				BestDiscoveryTime: firstLap.DiscoveryTimePrepared,
				BestLastSeenTime:  firstLap.LastSeenTimePrepared,
			}

			filtered <- averageLap //send to chan
			laps = []models.Lap{}  // make laps nil

		}
	}
}

func getAverageTime(laps []models.Lap) (bdt, blst time.Time) {
	for _, v := range laps {
		if bdt.IsZero() || blst.IsZero() {
			bdt = v.DiscoveryTimePrepared
			blst = v.LastSeenTimePrepared

			continue
		}

		bdt.Sub(v.DiscoveryTimePrepared)
		// sort
		// average

	}

	return
}
