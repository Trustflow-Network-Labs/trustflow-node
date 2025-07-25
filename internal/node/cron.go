package node

import (
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/robfig/cron"
)

type CronManager struct {
	p2pm *P2PManager
	cfgm *utils.ConfigManager
	lm   *utils.LogsManager
	tcm  *TopicAwareConnectionManager
}

func NewCronManager(p2pm *P2PManager) *CronManager {
	return &CronManager{
		p2pm: p2pm,
		cfgm: utils.NewConfigManager(""),
		lm:   p2pm.Lm,
		tcm:  p2pm.tcm,
	}
}

func (cm *CronManager) JobQueue() (*cron.Cron, error) {
	configs, err := cm.cfgm.ReadConfigs()
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}

	jm := NewJobManager(cm.p2pm)
	c := cron.New()
	err = c.AddFunc(configs["peer_discovery"], cm.p2pm.DiscoverPeers)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	err = c.AddFunc(configs["connection_health_check"], cm.p2pm.MaintainConnections)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	err = c.AddFunc(configs["connection_stats"], cm.tcm.GetConnectionStats)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	err = c.AddFunc(configs["peer_evaluation"], cm.tcm.PeersEvaluation)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	err = c.AddFunc(configs["process_job_queue"], jm.ProcessQueue)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	err = c.AddFunc(configs["request_job_status_update"], jm.RequestWorkflowJobsStatusUpdates)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return nil, err
	}
	c.Start()

	return c, nil
}
