package main

import (
	log "github.com/Sirupsen/logrus"
	consulapi "github.com/hashicorp/consul/api"
	consulwatch "github.com/hashicorp/consul/watch"
)

// Wait for lock to the Consul KV key.
// This will ensure we are the only master is holding a lock and registered
func (rc *resecConfig) WaitForLock() {
	log.Info("Trying to acquire leader lock")
	consulClient := rc.consulClient()
	sessionID, err := session(consulClient)
	if err != nil {
		rc.errCh <- err
	}

	lock, err := consulClient.LockOpts(&consulapi.LockOptions{
		Key:     rc.consulLockKey,
		Session: sessionID,
	})

	if err != nil {
		rc.errCh <- err
	}

	_, err = lock.Lock(rc.lockAbortCh)

	if err != nil {
		rc.errCh <- err
	}

	log.Info("Lock acquired")

	rc.lockCh <- lock

	rc.sessionId = sessionID

}

// Create a Consul session used for locks
func session(c *consulapi.Client) (string, error) {
	s := c.Session()
	se := &consulapi.SessionEntry{
		Name: "ReSeC",
		TTL:  "10s",
	}

	id, _, err := s.Create(se, nil)
	if err != nil {
		return "", err
	}

	return id, nil
}

func (rc *resecConfig) ServiceRegister(replication_role string) error {
	serviceInfo := &consulapi.AgentServiceRegistration{
		Tags:    []string{replication_role},
		Port:    rc.redisPort,
		Address: rc.redisHost,
		Name:    rc.consulServiceName,
		Check: &consulapi.AgentServiceCheck{
			TCP:      rc.redisAddr,
			Interval: "5s",
			Timeout:  "2s",
		},
	}

	err := rc.consulClient().Agent().ServiceRegister(serviceInfo)
	if err != nil {
		log.Error("consul Service registration failed", "error", err)
		return err
	}
	log.Info("registration with consul completed", "sinfo", serviceInfo)
	return err
}

func (rc *resecConfig) Watch() (err error) {
	params := map[string]interface{}{
		"type":        "service",
		"service":     rc.consulServiceName,
		"tag":         "master",
		"passingonly": true,
	}

	wp, err := consulwatch.Parse(params)
	if err != nil {
		log.Error("couldn't create a watch plan", "error", err)
		return err
	}

	wp.Handler = func(idx uint64, data interface{}) {
		switch srvcs := data.(type) {
		case []*consulapi.ServiceEntry:
			log.Debug("got an array of ServiceEntry", "srvcs", srvcs)
			rc.masterCh <- srvcs
		default:
			log.Debug("got an unknown interface", "srvcs", srvcs)
		}

	}
	go func() {
		if err := wp.Run(rc.consulClientConfig.Address); err != nil {
			log.Error("got an error watching for changes", "error", err)
		}
	}()

	//Check if we should quit
	//wait forever for a stop signal to happen
	go func() {
		for {
			select {
			case <-rc.stopWatchCh:
				log.Debug("Stopped the watch, cause i'm the master")
				wp.Stop()
				return
			default:
			}
		}
	}()

	return
}