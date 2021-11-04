package Models

/**
* This package module have some methods for storage RFID labels
* and store them into database
*/

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"sort"
	"errors"

	"chicha/Packages/Config"
	"chicha/Packages/Proxy"
)

// Buffer for new RFID requests
var laps []Lap

// channel lockers 
var lapsChannelBufferLocker = make(chan int, 1)
var lapsChannelDBLocker = make(chan int, 1)

// Check RFID mute timeout map
var rfidTimeoutMap map[string]time.Time

// Check RFID mute timeout locker
var rfidTimeoutLocker sync.Mutex

// Start antenna listener
func StartAntennaListener() {

	if Config.PROXY_ACTIVE == "true" {
		log.Println("Started tcp proxy restream to", Config.PROXY_HOST, "and port:", Config.PROXY_PORT)
	}

	//unlock buffer operations
	lapsChannelBufferLocker <- 0 //Put the initial value into the channel to unlock operations

	//unlock db operations
	lapsChannelDBLocker <- 0 //Put the initial value into the channel to unlock operations

	//spin forever go routine to save in db with some interval:
	go saveLapsBufferSimplyToDB()

	// Create RFID mute timeout
	rfidTimeoutMap = make(map[string]time.Time)

	// Start listener
	l, err := net.Listen("tcp", Config.APP_ANTENNA_LISTENER_IP)
	if err != nil {
		log.Panicln("Can't start the antenna listener", err)
	}
	defer l.Close()

	// Listen new connections
	for {
		conn, err := l.Accept()
		if err != nil {
			if err != io.EOF {
				log.Panicln("tcp connection error:", err)
			}
		}

		go newAntennaConnection(conn)
	}
}

func saveLapsBufferSimplyToDB() {

	//loop forever:
	for range time.Tick(time.Duration(Config.LAPS_SAVE_INTERVAL_SEC) * time.Second) {
		<-lapsChannelDBLocker //grab the ticket via channel (lock)
		//log.Println("Saving buffer to database started.")
		for _, lap := range laps {
			var newLap Lap
			err := DB.Where("race_id = ?", lap.RaceID).Where("lap_number = ?", lap.LapNumber).Where("tag_id = ?", lap.TagID).Where("discovery_unix_time = ?", lap.DiscoveryUnixTime).First(&newLap).Error
			//log.Printf("race_id = %d, lap_number = %d, tag_id = %s, discovery_unix_time = %d \n", lap.RaceID, lap.LapNumber, lap.TagID, lap.DiscoveryUnixTime);
			if err == nil {
				//found old data - just update it:

				//////////////////// DATA MAGIC START ///////////////////
				//newlap.ID //taken from DB (on save)
				newLap.OwnerID = lap.OwnerID
				newLap.TagID = lap.TagID
				newLap.DiscoveryUnixTime =  lap.DiscoveryUnixTime
				newLap.DiscoveryAverageUnixTime = lap.DiscoveryAverageUnixTime
				newLap.DiscoveryTimePrepared = lap.DiscoveryTimePrepared
				newLap.DiscoveryAverageTimePrepared = lap.DiscoveryAverageTimePrepared
				newLap.DiscoveryTimePrepared  = lap.DiscoveryTimePrepared
				newLap.AverageResultsCount = lap.AverageResultsCount
				newLap.Antenna  = lap.Antenna
				newLap.AntennaIP  = lap.AntennaIP
				newLap.UpdatedAt  = lap.UpdatedAt
				newLap.RaceID = lap.RaceID
				newLap.CurrentRacePosition = lap.CurrentRacePosition
				newLap.TimeBehindTheLeader = lap.TimeBehindTheLeader
				newLap.LapNumber = lap.LapNumber
				newLap.LapTime = lap.LapTime
				newLap.LapPosition = lap.LapPosition
				newLap.LapIsCurrent = lap.LapIsCurrent
				newLap.LapIsStrange = lap.LapIsStrange
				newLap.StageFinished = lap.StageFinished
				newLap.BestLapTime = lap.BestLapTime
				newLap.BestLapNumber = lap.BestLapNumber
				newLap.BestLapPosition = lap.BestLapPosition
				newLap.RaceTotalTime = lap.RaceTotalTime
				newLap.BetterOrWorseLapTime = lap.BetterOrWorseLapTime
				//////////////////// DATA MAGIC END ///////////////////

				err := DB.Save(&newLap).Error
				if err != nil {
					log.Println("Error. Not updated in database:", err)
				}
			} else {
				//log.Println("Data not found in database:", err)
				//not found - create new
				err := DB.Create(&lap).Error;
				if err != nil {
					log.Println("Error. Not created new data in database:", err)
				}
			}
		}

		lapsChannelDBLocker <- 1 //give ticket back via channel (unlock)
	}
}

//export laps slice to other packages
func GetLaps() (outLaps []Lap, err error) {
	//<-lapsChannelBufferLocker //grab the ticket via channel (lock)
	if len(laps) == 0 {
		//laps buffer empty - recreate last race from db:
		laps, err = GetCurrentRaceDataFromDB()
		if err == nil {
			outLaps = laps
		}
	}
	//lapsChannelBufferLocker <- 1 //give ticket back via channel (unlock)

	if len(laps) > 0 {
		outLaps = laps
	}

	return
}

func setMyPreviousLapsNonCurrentInBuffer(myNewLap Lap)  {
	//get my previous results from this race - start block.
	//var onlyMyLaps []Lap
	//gather all my laps from previous results:
	for i, lap := range laps {
		if lap.RaceID == myNewLap.RaceID && lap.TagID == myNewLap.TagID {
			//my previous results found in this race:
			laps[i].LapIsCurrent=0;
			//onlyMyLaps = append(onlyMyLaps, lap)
		}
	}
	//log.Printf("Found %d previous laps and set LapIsCurrent=0 on them.\n", len(onlyMyLaps))
}

func getMyLastLapFromBuffer(newLap Lap) (myLastLap Lap, err error) {

	if len(laps) != 0 {
		//block 1: get my previous results from this race - start block.
		var myLastLaps []Lap
		//gather all my laps from previous results:
		for _, savedLap := range laps {
			if savedLap.TagID == newLap.TagID {
				myLastLaps = append(myLastLaps, savedLap)
			}
		}

		if len(myLastLaps) != 0 {
			//allready have more than one lap
			sort.Slice(myLastLaps, func(i, j int) bool {
				//sort descending by DisoveryUnixTime
				return myLastLaps[i].DiscoveryUnixTime > myLastLaps[j].DiscoveryUnixTime
			})
			//get my last result (newest inverted DisoveryUnixTime result)
			myLastLap = myLastLaps[0]
		} else {
			err = errors.New("My results buffer empty.")
		}
	} else {
		err = errors.New("Laps buffer empty.")
	}
	return
	//block 1: get my previous results from this race - finish block.
}

func getLastLapFromBuffer() (lastLap Lap, err error) {
	//block 1: get previous results from this race - start block.

	if len(laps) != 0 { 

		//create copy lapsCopy
		var lapsCopy []Lap
		for _, lap := range laps {
			lapsCopy = append(lapsCopy, lap)
		}

		sort.Slice(lapsCopy, func(i, j int) bool {
			//sort descending by DisoveryUnixTime
			return lapsCopy[i].DiscoveryUnixTime > lapsCopy[j].DiscoveryUnixTime
		})
		lastLap = lapsCopy[0]

		//clean lapsCopy
		var cL []Lap
		lapsCopy = cL

		return
	} else {
		//retrun empty lap struct 
		err = errors.New("Error: laps buffer is empty.")
		return
	}
}

func getLastRaceIdFromBuffer() (raceID uint) {
	//block 1: get previous results from this race - start block.

	if len(laps) != 0 {
		lapsCopy := laps
		sort.Slice(lapsCopy, func(i, j int) bool {
			//sort descending by DisoveryUnixTime
			return lapsCopy[i].RaceID > lapsCopy[j].RaceID
		})
		raceID = lapsCopy[0].RaceID
		return
	} else {
		//retrun 0 raceID
		raceID = 0
		return
	}
}

func checkLapIsValid(myLap Lap) bool {

	if len(laps) == 0 {
		//все в порядке - кругов  еще нет - первый участник - вернем true
		return true
	} else {

		if myLap.LapNumber == 0 {
			//едем нулевой круг
			for _, lap := range laps {
				if lap.RaceID==myLap.RaceID && lap.LapNumber == 1 {
					//если кто то проехал уже 2 круга а мы едем только нулевой
					//не нормально - помечаем что круг странный (возможно не считалась метка)
					return false
				}
			}
		} else if myLap.LapNumber == 1 {
			//едем первый полный круг

			for _, lap := range laps {
				//узнаем лучшее время первого круга у других участников:
				if lap.RaceID == myLap.RaceID && lap.LapNumber == 1 && lap.CurrentRacePosition == 1 {

					//узнаем соотношение времени лучшего проезда первого круга к нашему времени
					lapTimeRatio := int(math.Round ( float64(lap.LapTime) / float64(myLap.LapTime) ) )

					if lapTimeRatio >= 2 {
						//если наше время в 2 или более раз долльше лучего времени этого круга у других участников
						//отметим что круг странный (возможно не считалась метка)
						return false
					}
				}
			}
		} else {
			//едем второй полный круг и все последующие

			for _, lap := range laps {
				//узнаем свое время предыдущего круга:
				if lap.RaceID == myLap.RaceID && lap.TagID == myLap.TagID && lap.LapNumber == ( myLap.LapNumber - 1 ) {

					//узнаем соотношение времени проезда своего предыдущего круга к текущему времени
					lapTimeRatio := int(math.Round ( float64(lap.LapTime) / float64(myLap.LapTime) ) )

					if lapTimeRatio >= 2 {
						//если наше время в 2 или более раз долльше нашего предыдущего
						//отметим что круг странный (возможно не считалась метка)
						return false
					}
				}
			}
		}

		//если ошибок не найдено - вернем true
		return true
	}
}


func getLapPositionFromBuffer(lastLap Lap) (lapPosition uint) {
	if len(laps) != 0  {
		var sameRoundLaps []Lap
		for _, savedLap := range laps {
			if savedLap.LapNumber == lastLap.LapNumber {
				sameRoundLaps = append(sameRoundLaps, savedLap)
			}
		}
		lapPosition = uint(len(sameRoundLaps) + 1 )
	} else {
		lapPosition = 1
	}

	return
}

func getZeroLapGap(lastLap Lap) (zeroLapGap int64) {

	if len(laps) != 0  {
		for _, lap := range laps {
			if lap.RaceID == lastLap.RaceID && lap.LapNumber == 0 && lap.CurrentRacePosition == 1 {
				zeroLapGap = lastLap.DiscoveryUnixTime - lap.DiscoveryUnixTime
			}
		}
	} else {
		zeroLapGap = 0
	}
	return
}

func containsTagID(laps []Lap, needle Lap) bool {
	for _, lap := range laps {
		if lap.TagID == needle.TagID {
			return true
		}
	}
	return false
}

func getMyBestLapTimeAndNumber(lastLap Lap) (myBestLapTime int64, myBestLapNumber int, myBestLapPosition uint) {
	if len(laps) != 0  {

		var myLaps []Lap

		for _, savedLap := range laps {
			if savedLap.RaceID == lastLap.RaceID && savedLap.TagID == lastLap.TagID && savedLap.LapNumber != 0 {
				myLaps = append(myLaps, savedLap)
			}
		}

		if lastLap.LapNumber != 0 {
			myLaps = append(myLaps, lastLap)
		}


		if len(myLaps) > 0 {
			sort.SliceStable(myLaps, func(i, j int) bool {
				return myLaps[i].LapTime < myLaps[j].LapTime
			})
			myBestLapTime = myLaps[0].LapTime
			myBestLapNumber = myLaps[0].LapNumber

		} else {
			myBestLapTime =  0
			myBestLapNumber = 0
		}

		//get position from all race laps 

		//get best laps from all racers
		var bestLaps []Lap
		for _, lap := range laps {
			if lap.RaceID == lastLap.RaceID && lap.BestLapTime != 0 && lap.LapNumber != 0 {
				bestLaps = append(bestLaps, lap)
			}
		}

		//add current lap
		if lastLap.LapNumber != 0 {
			lastLap.BestLapTime = myBestLapTime
			lastLap.BestLapNumber = myBestLapNumber
			bestLaps = append(bestLaps, lastLap)
		}

		if len(bestLaps) > 0 {
			sort.SliceStable(bestLaps, func(i, j int) bool {
				return bestLaps[i].BestLapTime < bestLaps[j].BestLapTime
			})
			var absoluteBestLaps []Lap
			for _, lap := range bestLaps {
				//create new slice with only results by TagID
				if !containsTagID(absoluteBestLaps, lap) {
					absoluteBestLaps = append(absoluteBestLaps, lap)
				}
			}

			if len(absoluteBestLaps) > 0 {
				for position, lap := range absoluteBestLaps {

					//prepare my best lap position
					if lap.TagID == lastLap.TagID {
						myBestLapPosition = uint(position + 1)
					}

					//update everyone else best lap position globally in current laps
					for i, _ := range laps {
						if laps[i].RaceID == lap.RaceID && laps[i].TagID == lap.TagID && laps[i].LapIsCurrent == 1 {
							laps[i].BestLapPosition = uint(position + 1)
						}
					}

					//log.Printf("Tag: %s, BestLapTime: %d, BestLapNumber: %d, BestLapPosition: %d\n", lap.TagID, lap.BestLapTime, lap.BestLapNumber, position+1)

				}
			}
		} else {
			myBestLapPosition = 0
		}

	} else {
		myBestLapTime = 0
		myBestLapNumber = 0
		myBestLapPosition = 0
	}

	return
}

func getTimeBehindTheLeader(lastLap Lap) (timeBehindTheLeader int64) {
	if len(laps) != 0  {

		var currentLaps []Lap

		for _, savedLap := range laps {
			if savedLap.LapIsCurrent == 1 {
				currentLaps = append(currentLaps, savedLap)
			}
		}

		lastLap.LapIsCurrent = 1
		currentLaps = append(currentLaps, lastLap)

		sort.Slice(currentLaps, func(i, j int) bool {
			if currentLaps[i].LapNumber != currentLaps[j].LapNumber {
				return currentLaps[i].LapNumber > currentLaps[j].LapNumber
			}
			return currentLaps[i].DiscoveryUnixTime < currentLaps[j].DiscoveryUnixTime
		})

		timeBehindTheLeader = lastLap.DiscoveryUnixTime - currentLaps[0].DiscoveryUnixTime
		//log.Printf("timeBehindTheLeader: %d = lastLap.DiscoveryUnixTime: %d - currentLaps[0].DiscoveryUnixTime: %d\n", timeBehindTheLeader, lastLap.DiscoveryUnixTime, currentLaps[0].DiscoveryUnixTime)

	} else {
		timeBehindTheLeader = 0
	}

	return
}


func getMyPreviousBestLapTime(lastLap Lap) (myPreviousBestLapTime int64) {

	if len(laps)!=0 {
		//get my previous BestLapTime
		var previousLaps []Lap

		for _, savedLap := range laps {
			if savedLap.RaceID == lastLap.RaceID && savedLap.TagID == lastLap.TagID && savedLap.LapNumber != lastLap.LapNumber && savedLap.LapNumber != 0  {
				previousLaps = append(previousLaps, savedLap)
			}
		}

		if len(previousLaps) > 0 {
			sort.SliceStable(previousLaps, func(i, j int) bool {
				return previousLaps[i].BestLapTime < previousLaps[j].BestLapTime
			})
			myPreviousBestLapTime = previousLaps[0].BestLapTime

		} else {
			myPreviousBestLapTime = 0
		}

	} else {
		myPreviousBestLapTime = 0
	}
	return
}

func getMyPreviousLapTime(lastLap Lap) (myPreviousLapTime int64) {

  if len(laps)!=0 {
    //get my previous LapTime
    var previousLaps []Lap

    for _, savedLap := range laps {
      if savedLap.RaceID == lastLap.RaceID && savedLap.TagID == lastLap.TagID && savedLap.LapNumber != lastLap.LapNumber && savedLap.LapNumber != 0  {
        previousLaps = append(previousLaps, savedLap)
      }
    }

    if len(previousLaps) > 0 {
      sort.SliceStable(previousLaps, func(i, j int) bool {
        return previousLaps[i].LapNumber > previousLaps[j].LapNumber
      })
      myPreviousLapTime = previousLaps[0].LapTime

    } else {
      myPreviousLapTime = 0
    }

  } else {
    myPreviousLapTime = 0
  }
  return
}


func getCurrentRacePositionFromBuffer(lastLap Lap) (currentRacePosition uint){

	if len(laps) != 0  {
		var currentLaps []Lap

		for _, savedLap := range laps {
			if savedLap.LapIsCurrent == 1 {
				currentLaps = append(currentLaps, savedLap)
			}
		}
		lastLap.LapIsCurrent = 1
		currentLaps = append(currentLaps, lastLap)

		sort.Slice(currentLaps, func(i, j int) bool {
			if currentLaps[i].LapNumber != currentLaps[j].LapNumber {
				return currentLaps[i].LapNumber > currentLaps[j].LapNumber
			}
			return currentLaps[i].DiscoveryUnixTime < currentLaps[j].DiscoveryUnixTime
		})

		for position, currentLap := range currentLaps {

			//set my current race position
			if  currentLap.TagID == lastLap.TagID {
				currentRacePosition = uint(position + 1)
			}
			//update other race positions:
			for i, lap := range laps {
				if lap.RaceID ==  currentLap.RaceID && lap.TagID == currentLap.TagID && lap.DiscoveryUnixTime == currentLap.DiscoveryUnixTime {
					laps[i].CurrentRacePosition=uint(position + 1)
				}
			}
		}

	} else {
		currentRacePosition = 1
	}

	return
}

// Add new lap to laps buffer (private func)
func addNewLapToLapsBuffer(newLap Lap) {
	<-lapsChannelBufferLocker //grab the ticket via channel (lock)
	newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond)
	if len(laps) == 0 {
		//empty data: create race and lap
		fmt.Println("Slice empty - adding new element with TagID = ", newLap.TagID)

		//FIRST RACE & FIRST LAP

		//////////////////// DATA MAGIC START ///////////////////
		//newlap.ID //taken from DB (on save)
		//newlap.OwnerID
		//newlap.TagID //taken from RFID
		newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond) //int64 
		newLap.DiscoveryAverageUnixTime = newLap.DiscoveryUnixTime //int64
		//newLap.DiscoveryTimePrepared //taken from RFID
		newLap.DiscoveryAverageTimePrepared = newLap.DiscoveryTimePrepared
		newLap.AverageResultsCount = 1 //first result from antenna

		//newLap.Antenna //taken from RFID
		//newLap.AntennaIP //taken from RFID
		newLap.UpdatedAt = time.Now() //current time in time.Time
		newLap.RaceID=1
		newLap.CurrentRacePosition=1
		newLap.TimeBehindTheLeader=0
		newLap.LapNumber=0
		newLap.LapTime=0
		newLap.LapPosition=1
		setMyPreviousLapsNonCurrentInBuffer(newLap)
		newLap.LapIsCurrent=1
		//newLap.LapIsStrange=0
		newLap.StageFinished=1
		newLap.BestLapTime=0
		newLap.BestLapNumber=0
		newLap.BestLapPosition=0
		newLap.RaceTotalTime=0
		newLap.BetterOrWorseLapTime=0
		if checkLapIsValid(newLap) {
			newLap.LapIsStrange=0
		} else {
			newLap.LapIsStrange=1
		}
		//////////////////// DATA MAGIC END ///////////////////

		laps = append(laps, newLap)
		log.Printf("SAVED %d TO BUFFER: laps: %d, raceid: %d, tag: %s, \n\n lap struct: %+v, \n\n laps slice: %+v\n\n", newLap.LapNumber, len(laps), newLap.RaceID,  newLap.TagID, newLap, laps )


	} else {
		//get any previous lap data:
		lastLap, err := getLastLapFromBuffer()
		if err != nil {
			//SOME ERROR - lastLap EMPTY ?
			log.Println("SOME ERROR - lastLap EMPTY:", err)
		} else {
			//lastLap not empty
			//get my previous lap data:
			lastGap := newLap.DiscoveryUnixTime - lastLap.DiscoveryUnixTime
			log.Println("lastGap:", lastGap)
			myLastLap, err := getMyLastLapFromBuffer(newLap)
			if err == nil {
				
				//существуют мои результаты в текущем заезде

				myLastGap := newLap.DiscoveryUnixTime - myLastLap.DiscoveryUnixTime
				fmt.Printf("myLastGap: %d \n", myLastGap)

				if  myLastGap >= -(Config.RESULTS_PRECISION_SEC*1000)  && myLastGap <= Config.RESULTS_PRECISION_SEC*1000  {
					//from -5sec to 5 sec (RESULTS_PRECISION_SEC)
					//результаты уже имеются - только обновить среднее и минимальное время 

					for i, lap := range laps {
						if lap.RaceID == myLastLap.RaceID && lap.TagID == myLastLap.TagID && lap.LapNumber == myLastLap.LapNumber && lap.DiscoveryUnixTime == myLastLap.DiscoveryUnixTime && lap.DiscoveryAverageUnixTime == myLastLap.DiscoveryAverageUnixTime {
							//get exact lap to update
							//UPDATE OTHER RESULTS IN BUFFER ACCORDING TO NEW TIME START BLOCK:

							//increment average results count
							laps[i].AverageResultsCount = laps[i].AverageResultsCount + 1

							//calculate and set new average time
							//unix:
							discoveryAverageUnixTime := (myLastLap.DiscoveryAverageUnixTime + newLap.DiscoveryUnixTime) / 2
							laps[i].DiscoveryAverageUnixTime = discoveryAverageUnixTime

							//time.Time:
							laps[i].DiscoveryAverageTimePrepared = timeFromUnixMillis(discoveryAverageUnixTime)

							//my stored time is older than in received new?
							if myLastLap.DiscoveryUnixTime > newLap.DiscoveryUnixTime {
								//set minimal(youngest) discovered time
								laps[i].DiscoveryUnixTime = newLap.DiscoveryUnixTime
								laps[i].DiscoveryTimePrepared = timeFromUnixMillis(newLap.DiscoveryUnixTime)
							}

							//laps[i].LapTime= myLastGap
							//laps[i].LapPosition=getLapPositionFromBuffer(laps[i])
							//laps[i].BestLapTime, laps[i].BestLapNumber, laps[i].BestLapPosition = getMyBestLapTimeAndNumber(laps[i])
							//laps[i].RaceTotalTime = myLastLap.RaceTotalTime + myLastGap
							//(-) minus is better (green), (+) plus is worth (orange).
							//laps[i].BetterOrWorseLapTime = laps[i].BestLapTime - getMyPreviousBestLapTime(laps[i])
							//laps[i].CurrentRacePosition=getCurrentRacePositionFromBuffer(laps[i])
							//laps[i].TimeBehindTheLeader=getTimeBehindTheLeader(laps[i])
							//if checkLapIsValid(laps[i]) {
							//	laps[i].LapIsStrange=0
							//} else {
							//	laps[i].LapIsStrange=1
							//}
							//UPDATE OTHER RESULTS IN BUFFER ACCORDING TO NEW TIME END BLOCK.


							//log.Printf("UPDATED BUFFER: raceid: %d, lap#: %d, results: %d, time: %d, avtime: %d, tag: %s\n", laps[i].RaceID, laps[i].LapNumber, laps[i].AverageResultsCount, laps[i].DiscoveryUnixTime, laps[i].DiscoveryAverageUnixTime, laps[i].TagID )
						}
					}

				} else if Config.RESULTS_PRECISION_SEC*1000 < myLastGap && myLastGap < Config.MINIMAL_LAP_TIME_SEC*1000 {
					//from 5 to 30 sec (RESULTS_PRECISION_SEC - MINIMAL_LAP_TIME_SEC) = discard data - ERROR DATA RECEIVED!

				log.Println("ERROR DATA RECEIVED: from 5 to 30 sec", newLap.TagID)


				} else if Config.MINIMAL_LAP_TIME_SEC*1000 <= myLastGap && lastGap < Config.RACE_TIMEOUT_SEC*1000 {
					//from 30 to 300 sec (MINIMAL_LAP_TIME_SEC - RACE_TIMEOUT_SEC) passed  = create new lap LapNumber++! 

					//////////////////// DATA MAGIC START ///////////////////
					//newlap.ID //taken from DB (on save)
					//newlap.OwnerID
					//newlap.TagID //taken from RFID
					newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond) //int64 
					newLap.DiscoveryAverageUnixTime = newLap.DiscoveryUnixTime //int64
					//newLap.DiscoveryTimePrepared //taken from RFID
					newLap.DiscoveryAverageTimePrepared = newLap.DiscoveryTimePrepared
					newLap.AverageResultsCount = 1 //first result from antenna

					//newLap.Antenna //taken from RFID
					//newLap.AntennaIP //taken from RFID
					newLap.UpdatedAt = time.Now() //current time in time.Time
					newLap.RaceID=myLastLap.RaceID
					//newLap.CurrentRacePosition=getCurrentRacePositionFromBuffer(newLap) - calculate at last
					//newLap.TimeBehindTheLeader=getTimeBehindTheLeader(newLap) - calculate at last
					newLap.LapNumber = myLastLap.LapNumber + 1
					newLap.LapTime= myLastGap
					newLap.LapPosition=getLapPositionFromBuffer(newLap)
					setMyPreviousLapsNonCurrentInBuffer(newLap)
					newLap.LapIsCurrent = 1 //returns 1 (sets this lap current) && sets my previous laps in same race: LapIsCurrent=0
					//newLap.LapIsStrange=?
					newLap.StageFinished=1
					newLap.BestLapTime, newLap.BestLapNumber, newLap.BestLapPosition = getMyBestLapTimeAndNumber(newLap)
					//newLap.BestLapPosition=getMyBestLapPosition(newLap)
					newLap.RaceTotalTime = myLastLap.RaceTotalTime + myLastGap
					if newLap.LapNumber == 1 {
						//first full lap - no BetterOrWorseLapTime data
						newLap.BetterOrWorseLapTime = 0
					} else {
						//(-) minus is better (green), (+) plus is worth (orange).
						newLap.BetterOrWorseLapTime = newLap.LapTime - getMyPreviousLapTime(newLap)
					}
					newLap.CurrentRacePosition=getCurrentRacePositionFromBuffer(newLap)
					newLap.TimeBehindTheLeader=getTimeBehindTheLeader(newLap)
					if checkLapIsValid(newLap) {
						newLap.LapIsStrange=0
					} else {
						newLap.LapIsStrange=1
					}
					//////////////////// DATA MAGIC END ///////////////////


					laps = append(laps, newLap)
					//log.Printf("ADDED NEXT LAP %d TO BUFFER: laps: %d, raceid: %d, tag: %s, \n\n lap struct: %+v, \n\n laps slice: %+v\n\n", newLap.LapNumber, len(laps), newLap.RaceID,  newLap.TagID, newLap, laps )

				} else if lastGap > Config.RACE_TIMEOUT_SEC*1000 {

					//rider data available, but race expired - create new race and append this rider.

					//////////////////// DATA MAGIC START ///////////////////
					//newlap.ID //taken from DB (on save)
					//newlap.OwnerID
					//newlap.TagID //taken from RFID
					newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond) //int64
					newLap.DiscoveryAverageUnixTime = newLap.DiscoveryUnixTime //int64
					//newLap.DiscoveryTimePrepared //taken from RFID
					newLap.DiscoveryAverageTimePrepared = newLap.DiscoveryTimePrepared
					newLap.AverageResultsCount = 1 //first result from antenna

					//newLap.Antenna //taken from RFID
					//newLap.AntennaIP //taken from RFID
					newLap.UpdatedAt = time.Now() //current time in time.Time
					newLap.RaceID=getLastRaceIdFromBuffer()+1
					newLap.CurrentRacePosition=1
					newLap.TimeBehindTheLeader=0
					newLap.LapNumber=0
					newLap.LapTime=0
					newLap.LapPosition=1
					setMyPreviousLapsNonCurrentInBuffer(newLap)
					newLap.LapIsCurrent=1
					//newLap.LapIsStrange=0
					newLap.StageFinished=1
					newLap.BestLapTime=0
					newLap.BestLapNumber=0
					newLap.BestLapPosition=0
					newLap.RaceTotalTime=0
					newLap.BetterOrWorseLapTime=0
					if checkLapIsValid(newLap) {
						newLap.LapIsStrange=0
					} else {
						newLap.LapIsStrange=1
					}
					//////////////////// DATA MAGIC END ///////////////////

					// Clear lap buffer, start with clean slice:
					var cL []Lap
					laps = cL
					laps = append(laps, newLap)

					//log.Printf("SAVED NEXT RACE LAP %d TO BUFFER: laps: %d, raceid: %d, tag: %s, \n\n lap struct: %+v, \n\n laps slice: %+v\n\n", newLap.LapNumber, len(laps), newLap.RaceID,  newLap.TagID, newLap, laps )



				} else {

					//log.Printf("STRANGE!: laps: %d, raceid: %d, lap#: %d, tag: %s\n", len(laps), newLap.RaceID, newLap.LapNumber, newLap.TagID )
				}
			} else {

				//no previous rider data in this race

				if lastGap > Config.RACE_TIMEOUT_SEC*1000 {
					
					//rece expired - create new and append new rider

					//////////////////// DATA MAGIC START ///////////////////
					//newlap.ID //taken from DB (on save)
					//newlap.OwnerID
					//newlap.TagID //taken from RFID
					newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond) //int64
					newLap.DiscoveryAverageUnixTime = newLap.DiscoveryUnixTime //int64
					//newLap.DiscoveryTimePrepared //taken from RFID
					newLap.DiscoveryAverageTimePrepared = newLap.DiscoveryTimePrepared
					newLap.AverageResultsCount = 1 //first result from antenna

					//newLap.Antenna //taken from RFID
					//newLap.AntennaIP //taken from RFID
					newLap.UpdatedAt = time.Now() //current time in time.Time
					newLap.RaceID=getLastRaceIdFromBuffer()+1
					newLap.CurrentRacePosition=1
					newLap.TimeBehindTheLeader=0
					newLap.LapNumber=0
					newLap.LapTime=0
					newLap.LapPosition=1
					setMyPreviousLapsNonCurrentInBuffer(newLap)
					newLap.LapIsCurrent=1
					//newLap.LapIsStrange=0
					newLap.StageFinished=1
					newLap.BestLapTime=0
					newLap.BestLapNumber=0
					newLap.BestLapPosition=0
					newLap.RaceTotalTime=0
					newLap.BetterOrWorseLapTime=0
					if checkLapIsValid(newLap) {
						newLap.LapIsStrange=0
					} else {
						newLap.LapIsStrange=1
					}
					//////////////////// DATA MAGIC END ///////////////////

					// Clear lap buffer, start with clean slice:
					var cL []Lap
					laps = cL
					laps = append(laps, newLap)

					//log.Printf("CREATED NEW RACE %d, with lap#  %d IN BUFFER: tag: %s, \n", newLap.RaceID, newLap.LapNumber, newLap.TagID)

				} else {
					
					//race is valid - append new rider

					//////////////////// DATA MAGIC START ///////////////////
					//newlap.ID //taken from DB (on save)
					//newlap.OwnerID
					//newlap.TagID //taken from RFID
					newLap.DiscoveryUnixTime = newLap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond) //int64
					newLap.DiscoveryAverageUnixTime = newLap.DiscoveryUnixTime //int64
					//newLap.DiscoveryTimePrepared //taken from RFID
					newLap.DiscoveryAverageTimePrepared = newLap.DiscoveryTimePrepared
					newLap.AverageResultsCount = 1 //first result from antenna

					//newLap.Antenna //taken from RFID
					//newLap.AntennaIP //taken from RFID
					newLap.UpdatedAt = time.Now() //current time in time.Time
					newLap.RaceID=lastLap.RaceID
					//newLap.CurrentRacePosition=getCurrentRacePositionFromBuffer(newLap) - calculate at last
					//newLap.TimeBehindTheLeader=getTimeBehindTheLeader(newLap) - calc at last
					newLap.LapNumber = 0
					newLap.LapTime = getZeroLapGap(newLap)
					newLap.LapPosition=getLapPositionFromBuffer(newLap)

					setMyPreviousLapsNonCurrentInBuffer(newLap)
					newLap.LapIsCurrent = 1
					//newLap.LapIsStrange=?
					newLap.StageFinished=1
					newLap.BestLapTime, newLap.BestLapNumber, newLap.BestLapPosition = getMyBestLapTimeAndNumber(newLap)
					//newLap.BestLapPosition=?
					newLap.RaceTotalTime = newLap.LapTime
					if newLap.LapNumber == 1 {
						//first full lap - no BetterOrWorseLapTime data
						newLap.BetterOrWorseLapTime = 0
					} else {
						//(-) minus is better (green), (+) plus is worth (orange).
						newLap.BetterOrWorseLapTime = newLap.LapTime - getMyPreviousLapTime(newLap) 
					}
					newLap.CurrentRacePosition=getCurrentRacePositionFromBuffer(newLap)
					newLap.TimeBehindTheLeader=getTimeBehindTheLeader(newLap)
					if checkLapIsValid(newLap) {
						newLap.LapIsStrange=0
					} else {
						newLap.LapIsStrange=1
					}
					//////////////////// DATA MAGIC END ///////////////////

					laps = append(laps, newLap)

					//log.Printf("SAVED NEW PLAYER TO SAME RACE in BUFFER:  LAP %d TO BUFFER: laps: %d, raceid: %d, tag: %s, \n\n lap struct: %+v, \n\n laps slice: %+v\n\n", newLap.LapNumber, len(laps), newLap.RaceID,  newLap.TagID, newLap, laps )


				}



			}
		}
	}

	for _, lap := range laps {
		if lap.LapIsCurrent==1 {
			fmt.Printf("lap: %d, tag: %s, position: %d, start#: %d, time: %d, gap: %d, best lap: %d, alive?: %d, strange?: %d\n", lap.LapNumber, lap.TagID, lap.CurrentRacePosition, lap.BestLapPosition, lap.RaceTotalTime, lap.TimeBehindTheLeader, lap.BestLapTime, lap.StageFinished, lap.LapIsStrange)
		}
	}

	lapsChannelBufferLocker <- 1 //give ticket back via channel (unlock)

}
// Set new expired date for rfid Tag
func setNewExpriredDataForRfidTag(tagID string) {

	newExpiredTime := time.Now().Add(time.Duration(Config.MINIMAL_LAP_TIME_SEC) * time.Second)
	rfidTimeoutLocker.Lock()
	rfidTimeoutMap[tagID] = newExpiredTime
	rfidTimeoutLocker.Unlock()

}

//string to time.Unix milli
func timeFromUnixMillis(msInt int64) (time.Time) {
	return time.Unix(0, msInt*int64(time.Millisecond))
}


//string to time.Unix milli
func timeFromStringUnixMillis(ms string) (time.Time, error) {
	msInt, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(0, msInt*int64(time.Millisecond)), nil
}

func IsValidXML(data []byte) bool {
	return xml.Unmarshal(data, new(interface{})) == nil
}

// New antenna connection (private func)
func newAntennaConnection(conn net.Conn) {
	defer conn.Close()
	var tempDelay time.Duration // how long to sleep on accept failure

	// Read connection in lap
	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				//log.Println("conn.Read(buf) error:", err)
				//log.Println("Message EOF detected - closing LAN connection.")
				break
			}

			if ne, ok := err.(*net.OpError); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Printf("http: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}

			break
		}
		tempDelay = 0

		data := buf[:size]
		var lap Lap
		lap.AntennaIP = fmt.Sprintf("%s", conn.RemoteAddr().(*net.TCPAddr).IP)

		//various data formats processing (text csv, xml) start:
		if !IsValidXML(data) {
			// CSV data processing
			//log.Println("Received data is not XML, trying CSV text...")
			//received data of type TEXT (parse TEXT).
			r := csv.NewReader(bytes.NewReader(data))
			r.Comma = ','
			r.FieldsPerRecord = 3
			CSV, err := r.Read()
			if err != nil {
				log.Println("Recived incorrect CSV data", err)
				continue
			}

			// Prepare antenna position
			antennaPosition, err := strconv.ParseInt(strings.TrimSpace(CSV[2]), 10, 64)
			if err != nil {
				log.Println("Recived incorrect Antenna position CSV value:", err)
				continue
			}
			_, err = strconv.ParseInt(strings.TrimSpace(CSV[1]), 10, 64)
			if err != nil {
				log.Println("Recived incorrect discovery unix time CSV value:", err)
				continue
			} else {
				lap.DiscoveryTimePrepared, _ = timeFromStringUnixMillis(strings.TrimSpace(CSV[1]))
			}
			lap.TagID = strings.TrimSpace(CSV[0])
			lap.Antenna = uint8(antennaPosition)

			// XML data processing
		} else {
			// XML data processing
			// Prepare date
			//log.Println("Received data is valid XML")
			err := xml.Unmarshal(data, &lap)
			if err != nil {
				log.Println("xml.Unmarshal ERROR:", err)
				continue
			}
			//log.Println("TIME_ZONE=", Config.TIME_ZONE)
			loc, err := time.LoadLocation(Config.TIME_ZONE)
			if err != nil {
				log.Println(err)
				continue
			}
			xmlTimeFormat := `2006/01/02 15:04:05.000`
			discoveryTime, err := time.ParseInLocation(xmlTimeFormat, lap.DiscoveryTime, loc)
			if err != nil {
				log.Println("time.ParseInLocation ERROR:", err)
				continue
			}
			lap.DiscoveryTimePrepared = discoveryTime
			// Additional preparing for TagID
			lap.TagID = strings.ReplaceAll(lap.TagID, " ", "")
		}
		//various data formats processing (text csv, xml) end.

		//Debug all received data from RFID reader
		log.Printf("NEW: IP=%s, TAG=%s, TIME=%d, ANT=%d\n", lap.AntennaIP, lap.TagID, lap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond), lap.Antenna)

		if Config.PROXY_ACTIVE == "true" {
			go Proxy.ProxyDataToMotosponder(lap.TagID, lap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond), lap.Antenna)
		}


		<-lapsChannelBufferLocker //grab the ticket via channel (lock)
		if len(laps) == 0 {
			//laps buffer empty - recreate last race from db:
			log.Println("laps buffer empty - recreate last race from db")
			laps, err = GetCurrentRaceDataFromDB()
			if err == nil {
				log.Printf("laps buffer recreated with %d records from db.\n", len(laps))
				go addNewLapToLapsBuffer(lap)
			} else {
				log.Println("laps buffer recreation failed with:", err)
				go addNewLapToLapsBuffer(lap)
			}
		} 
		lapsChannelBufferLocker <- 1 //give ticket back via channel (unlock)

		if len(laps) > 0 {
			// Add current Lap to Laps buffer
			go addNewLapToLapsBuffer(lap)
		}
	}
}
