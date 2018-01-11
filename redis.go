package main

import (
	"log"
	"strconv"
	"time"
)

func (rc *resecConfig) runAsMaster() {
	for {
		if promote := <-rc.promoteCh; promote {
			rc.stopWatchCh <- struct{}{}

			rc.promote()
		}
	}

}

func (rc *resecConfig) runAsSlave() {

	for {
		currentMaster := <-rc.masterConsulServiceCh
		log.Println("[INFO] Ok, There's a healthy master in consul, I'm obeying this!")

		// Use master node address if it's registered without service address
		var masterAddress string
		if currentMaster.Service.Address != "" {
			masterAddress = currentMaster.Service.Address
		} else {
			masterAddress = currentMaster.Node.Address
		}

		enslaveErr := rc.redisClient.SlaveOf(masterAddress, strconv.Itoa(currentMaster.Service.Port)).Err()

		if enslaveErr != nil {
			log.Printf("[ERROR] Failed to enslave redis to %s:%d", masterAddress, currentMaster.Service.Port)
		}

		rc.serviceRegister("slave")
	}
}

func (rc *resecConfig) promote() {
	promoteErr := rc.redisClient.SlaveOf("no", "one").Err()

	if promoteErr != nil {
		log.Printf("[ERROR] Failed to promote  redis to master - %s", promoteErr)
	} else {
		rc.master = true
		rc.serviceRegister("master")
		log.Println("[INFO] Promoted redis to Master")
	}

}

func (rc *resecConfig) redisHealthCheck() {

	for rc.redisMonitorEnabled {

		result, err := rc.redisClient.Info("replication").Result()

		if err != nil {
			log.Printf("[ERROR] Can't connect to redis running on %s", rc.redisAddr)

			rc.redisHealthCh <- &redisHealth{
				Output:  "",
				Healthy: false,
			}
			//if rc.waitingForLock {
			//	rc.lockAbortCh <- struct{}{}
			//	rc.waitingForLock = false
			//}
			//rc.redisHealthy = false
			//rc.master = false
		} else {
			rc.redisHealthCh <- &redisHealth{
				Output:  result,
				Healthy: true,
			}

			//rc.redisHealthy = true
			//if rc.consulCheckId != "" {
			//	log.Printf("[DEBUG] Redis health OK")
			//	err = rc.consulClient.Agent().UpdateTTL(rc.consulCheckId, result, "pass")
			//	if err != nil {
			//		log.Printf("[ERROR] Failed to update consul Check TTL - %s", err)
			//	}
			//}
			//if !rc.master {
			//	go rc.waitForLock()
			//}
		}

		time.Sleep(rc.healthCheckInterval)
	}

}