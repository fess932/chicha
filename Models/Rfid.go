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

  "chicha/Packages/Config"
  "chicha/Packages/Proxy"
)

// Buffer for new RFID requests
var laps []Lap

// Laps locker
var lapsLocker sync.Mutex

// Laps save into DB interval
var lapsSaveInterval int

// Check RFID mute timeout map
var rfidTimeoutMap map[string]time.Time

// Mute timeout duration (stored in .env)
var rfidLapMinimalTime int

// Check RFID mute timeout locker
var rfidTimeoutLocker sync.Mutex

// Start antenna listener
func StartAntennaListener() {

  if Config.PROXY_ACTIVE == "true" {
    fmt.Println("Started tcp proxy restream to", Config.PROXY_HOST, "and port:", Config.PROXY_PORT)
  }

  // Start buffer synchro with database
  go startSaveLapsBufferToDatabase()

  // Create RFID mute timeout
  rfidTimeoutMap = make(map[string]time.Time)

  // Prepare rfidLapMinimalTime
  rfidTimeout, rfidTimeoutErr := strconv.ParseInt(Config.MINIMAL_LAP_TIME, 10, 64)
  if rfidTimeoutErr != nil {
    log.Panicln("Incorrect MINIMAL_LAP_TIME parameter in .env file", rfidTimeoutErr)
  }
  rfidLapMinimalTime = int(rfidTimeout)

  // Prepare lapsSaveInterval
  lapsInterval, lapsIntervalErr := strconv.ParseInt(Config.LAPS_SAVE_INTERVAL, 10, 64)
  if lapsIntervalErr != nil {
    log.Panicln("Incorrect LAPS_SAVE_INTERVAL parameter in .env file", lapsIntervalErr)
  }
  lapsSaveInterval = int(lapsInterval)

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

// Save laps buffer to database
func startSaveLapsBufferToDatabase() {
  for range time.Tick(time.Duration(lapsSaveInterval) * time.Second) {
    lapsLocker.Lock()
    var lapStruct Lap
    var currentlapRaceID uint
    var currentlapLapNumber int
    lastRaceID, lastLapTime := GetLastRaceIDandTime(&lapStruct)
    if lastRaceID == 0 {
      currentlapRaceID = 1
    } else {

      raceTimeOut, _ := strconv.ParseInt(Config.RACE_TIMEOUT_SEC, 10, 64)
      if time.Now().UnixNano()/int64(time.Millisecond)-(int64(raceTimeOut)*1000) > lastLapTime.UnixNano()/int64(time.Millisecond) {
	//last lap data was created more than RACE_TIMEOUT_SEC seconds ago
	//RaceID++ (create new race)
	currentlapRaceID = lastRaceID + 1

      } else {
	//last lap data was created less than RACE_TIMEOUT_SEC seconds ago
	currentlapRaceID = lastRaceID
      }
    }

    // Save laps to database
    for _, lap := range laps {
      previousLapNumber, previousDiscoveryUnixTime, previousRaceTotalTime := GetPreviousLapDataFromRaceByTagID(lap.TagID, currentlapRaceID)
      if previousLapNumber != -1 {
	//set lap.LapIsCurrent = 0 for previous lap
	//set previos lap "non current"
	ExpireMyPreviousLap(lap.TagID, currentlapRaceID)
      }
      if previousLapNumber == -1 {
	currentlapLapNumber = 0
      } else {
	currentlapLapNumber = previousLapNumber + 1
      }
      //set this lap actual (current)
      lap.LapIsCurrent = 1
      lap.LapNumber = currentlapLapNumber
      lap.RaceID = currentlapRaceID
      lap.DiscoveryUnixTime = lap.DiscoveryTimePrepared.UnixNano() / int64(time.Millisecond)
      if previousLapNumber == -1 {
	//if this is first lap results:
	//#7 issue - first lap time
	leaderFirstLapDiscoveryUnixTime, err := GetLeaderFirstLapDiscoveryUnixTime(currentlapRaceID)
	if err == nil {
	  //you are not the leader of the first lap
	  //calculate against the leader
	  lap.LapTime = lap.DiscoveryUnixTime - leaderFirstLapDiscoveryUnixTime
	  lap.LapPosition = GetLapPosition(currentlapRaceID, currentlapLapNumber, lap.TagID)
	} else {
	  //you are the leader set LapTime=0;
	  lap.LapTime = 0
	  lap.LapPosition = 1
	  lap.CurrentRacePosition = 1
	}
      } else {
	lap.LapTime = lap.DiscoveryUnixTime - previousDiscoveryUnixTime
	lap.LapPosition = GetLapPosition(currentlapRaceID, currentlapLapNumber, lap.TagID)
      }

      //race total time
      lap.RaceTotalTime = previousRaceTotalTime + lap.LapTime
      //fmt.Println("race total time:", lap.RaceTotalTime, "lap time", lap.LapTime)

      leaderRaceTotalTime := GetLeaderRaceTotalTimeByRaceIdAndLapNumber(lap.RaceID, lap.LapNumber)
      if leaderRaceTotalTime == 0 {
	//first lap
	//fmt.Println("leaderRaceTotalTime = 0 - first lap detected, TimeBehindTheLeader = lap.LapTime:", lap.LapTime)
	if lap.LapPosition == 1 {
	  lap.TimeBehindTheLeader = 0
	} else {
	  lap.TimeBehindTheLeader = lap.LapTime
	}
      } else {
	lap.TimeBehindTheLeader = lap.RaceTotalTime - leaderRaceTotalTime
      }

      //START: лучшее время и возможные пропуски в учете на воротах RFID (lap.LapIsStrange):
      if lap.LapNumber == 0 {
	//едем нулевой круг
	lap.BestLapTime = lap.LapTime
	lap.BetterOrWorseLapTime = 0
	_, err := GetBestLapTimeFromRace(currentlapRaceID)
	if err == nil {
	  //если кто то проехал уже 2 круга а мы едем только нулевой
	  //не нормально - помечаем что круг странный (возможно не считалась метка)
	  lap.LapIsStrange = 1
	} else {
	  //нормально - еще нет проехавших второй круг
	  lap.LapIsStrange = 0
	}
      } else if lap.LapNumber == 1 {
	//едем первый полный круг
	lap.BestLapTime = lap.LapTime
	lap.BetterOrWorseLapTime = 0
	//узнаем лучшее время круга у других участников:
	currentRaceBestLapTime, _ := GetBestLapTimeFromRace(currentlapRaceID)
	lapIsStrange := int(math.Round(float64(lap.LapTime) / float64(currentRaceBestLapTime)))
	if lapIsStrange >= 2 {
	  //если наше время в 2 или более раз долльше лучего времени этого круга у других участников
	  //отметим что круг странный (возможно не считалась метка)
	  lap.LapIsStrange = 1
	} else {
	  //нормально - наше время не очень долгое (вероятно правильно считалось)
	  lap.LapIsStrange = 0
	}
      } else {
	//едем второй полный круг и все последующие
	//запросим свое предыдущее лучшее время круга:
	myPreviousBestLapTime, _ := GetBestLapTimeFromRaceByTagID(lap.TagID, currentlapRaceID)
	if lap.LapTime > myPreviousBestLapTime {
	  lap.BestLapTime = myPreviousBestLapTime
	} else {
	  lap.BestLapTime = lap.LapTime
	}
	//улучшил или ухудшил свое предыдущее лучшее время?
	lap.BetterOrWorseLapTime = lap.LapTime - myPreviousBestLapTime
	lapIsStrange := int(math.Round(float64(lap.LapTime) / float64(lap.BestLapTime)))
	if lapIsStrange >= 2 {
	  //если наше время в 2 и более раз дольше чем наше лучшее время круга
	  //отметим что круг странный (метка возможно просто не считалась)
	  lap.LapIsStrange = 1
	} else {
	  lap.LapIsStrange = 0
	}
      }
      //END: лучшее время и возможные пропуски в учете на воротах RFID (lap.LapIsStrange):

      err := DB.Create(&lap).Error
      if err != nil {
	fmt.Println("Error. Lap not added to database", err)
      } else {
	fmt.Printf("Saved! tag: %s, lap: %d, lap time: %d, total time: %d \n", lap.TagID, lap.LapNumber, lap.LapTime, lap.RaceTotalTime)
	spErr := UpdateCurrentStartPositionsByRaceId(currentlapRaceID)
	if spErr != nil {
	  fmt.Println("UpdateCurrentStartPositionsByRaceId(currentlapRaceID) Error", spErr)
	}
	upErr := UpdateCurrentResultsByRaceId(currentlapRaceID)
	if upErr != nil {
	  fmt.Println("UpdateCurrentResultsByRaceId(currentlapRaceID) Error", upErr)
	}

	//refresh my results
	golERR := DB.Where("id = ?", lap.ID).First(&lap).Error
	if golERR == nil {
	  if lap.CurrentRacePosition == 1 {
	    //if I am the leader - update other riders results - set lap.StageFinished=0
	    err := UpdateAllStageNotYetFinishedByRaceId(currentlapRaceID)
	    if err != nil {
	      fmt.Println("UpdateAllStageNotYetFinishedByRaceId(currentlapRaceID) ERROR:", err)
	    }
	  }

	  //update that your lap is finished lap.StageFinished=1 in any case
	  lap.StageFinished = 1

	  //save final results
	  sfErr := DB.Save(&lap).Error
	  if sfErr != nil {
	    fmt.Println("lap.StageFinished=1 Error. Lap not added to database", sfErr)
	  } else {
	    err := PrintCurrentResultsByRaceId(currentlapRaceID)
	    if err != nil {
	      fmt.Println("PrintCurrentResultsByRaceId(currentlapRaceID) ERROR:", err)
	    }
	  }
	} else {
	  fmt.Println("GetOneLap(&lap) ERROR:", golERR)
	}
      }
    }

    // Clear lap buffer
    var cL []Lap
    laps = cL
    lapsLocker.Unlock()

  }
}

// Add new lap to laps buffer (private func)
func addNewLapToLapsBuffer(lap Lap) {

  // Check minimal lap time (we save only laps grater than MINIMAL_LAP_TIME from .env file)

  if expiredTime, ok := rfidTimeoutMap[lap.TagID]; !ok {

    // First time for this TagID, save lap to buffer
    lapsLocker.Lock()
    laps = append(laps, lap)
    lapsLocker.Unlock()

    // Add new value to timeouts checker map
    setNewExpriredDataForRfidTag(lap.TagID)

  } else {

    // Check previous time
    tN := time.Now()
    if tN.After(expiredTime) {

      // Time is over, save lap to buffer
      lapsLocker.Lock()
      laps = append(laps, lap)
      lapsLocker.Unlock()

      // Generate new expired time
      setNewExpriredDataForRfidTag(lap.TagID)

    }
  }
}

// Set new expired date for rfid Tag
func setNewExpriredDataForRfidTag(tagID string) {

  newExpiredTime := time.Now().Add(time.Duration(rfidLapMinimalTime) * time.Second)
  rfidTimeoutLocker.Lock()
  rfidTimeoutMap[tagID] = newExpiredTime
  rfidTimeoutLocker.Unlock()

}

//string to time.Unix milli
func timeFromUnixMillis(ms string) (time.Time, error) {
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
	//fmt.Println("conn.Read(buf) error:", err)
	//fmt.Println("Message EOF detected - closing LAN connection.")
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
    //fmt.Println("IP:", lap.AntennaIP)
    // CSV data processing
    if !IsValidXML(data) {
      //fmt.Println("Received data is not XML, trying CSV text...")
      //received data of type TEXT (parse TEXT).
      r := csv.NewReader(bytes.NewReader(data))
      r.Comma = ','
      r.FieldsPerRecord = 3
      CSV, err := r.Read()
      if err != nil {
	fmt.Println("Recived incorrect CSV data", err)
	continue
      }

      // Prepare antenna position
      antennaPosition, err := strconv.ParseInt(strings.TrimSpace(CSV[2]), 10, 64)
      if err != nil {
	fmt.Println("Recived incorrect Antenna position CSV value:", err)
	continue
      }
      _, err = strconv.ParseInt(strings.TrimSpace(CSV[1]), 10, 64)
      if err != nil {
	fmt.Println("Recived incorrect discovery unix time CSV value:", err)
	continue
      } else {
	lap.DiscoveryTimePrepared, _ = timeFromUnixMillis(strings.TrimSpace(CSV[1]))
      }
      lap.TagID = strings.TrimSpace(CSV[0])
      lap.Antenna = uint8(antennaPosition)

      // XML data processing
    } else {
      // XML data processing
      // Prepare date
      //fmt.Println("Received data is valid XML")
      err := xml.Unmarshal(data, &lap)
      if err != nil {
	fmt.Println("xml.Unmarshal ERROR:", err)
	continue
      }
      //fmt.Println("TIME_ZONE=", Config.TIME_ZONE)
      loc, err := time.LoadLocation(Config.TIME_ZONE)
      if err != nil {
	fmt.Println(err)
	continue
      }
      xmlTimeFormat := `2006/01/02 15:04:05.000`
      discoveryTime, err := time.ParseInLocation(xmlTimeFormat, lap.DiscoveryTime, loc)
      if err != nil {
	fmt.Println("time.ParseInLocation ERROR:", err)
	continue
      }
      lap.DiscoveryTimePrepared = discoveryTime
      // Additional preparing for TagID
      lap.TagID = strings.ReplaceAll(lap.TagID, " ", "")
    }

    //Debug all received data from RFID reader
    fmt.Printf("NEW DATA from IP %s - %s, %d, %d\n", lap.AntennaIP, lap.TagID, lap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond), lap.Antenna)

    if Config.PROXY_ACTIVE == "true" {
      go Proxy.ProxyDataToMotosponder(lap.TagID, lap.DiscoveryTimePrepared.UnixNano()/int64(time.Millisecond), lap.Antenna)
    }
    // Add current Lap to Laps buffer
    go addNewLapToLapsBuffer(lap)
  }
}
