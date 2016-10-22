package artnet

import (
	"net"
	"time"
)

type Discovery struct {
	Nodes map[string]*Node
}

func (controller *Controller) discoveryPoll() error {
	for _, addr := range controller.discoveryAddrs {
		if err := controller.transport.SendPoll(addr); err != nil {
			return err
		} else {
			controller.log.Debugf("discovery: sendPoll %v", addr)
		}
	}

	return nil
}

func (controller *Controller) discovery(pollChan chan pollEvent) {
	var ticker = time.NewTicker(controller.config.DiscoveryInterval)
	var nodes = make(map[string]*Node)

	if err := controller.discoveryPoll(); err != nil {
		controller.log.Fatalf("discovery: sendPoll: %v", err)
	}

	for {
		select {
		case t := <-ticker.C:
			// scan timeouts
			var timeout = false

			for name, node := range nodes {
				if dt := t.Sub(node.discoveryTime); dt > node.timeout {
					controller.log.Warnf("discovery timeout: %v", node)

					delete(nodes, name)

					timeout = true
				}
			}

			if timeout {
				controller.update(nodes)
			}

			// poll
			controller.log.Debug("discovery: tick...")

			if err := controller.discoveryPoll(); err != nil {
				controller.log.Fatalf("discovery: sendPoll: %v", err)
			}

		case pollEvent := <-pollChan:
			nodeConfig := pollEvent.packet.NodeConfig()

			if node := nodes[pollEvent.String()]; node != nil {
				node.discoveryTime = pollEvent.recvTime
				node.config = nodeConfig // XXX: atomic

				controller.log.Debugf("discovery refresh: %v", node)

			} else if node, err := controller.makeNode(pollEvent.srcAddr, nodeConfig); err != nil {
				controller.log.Warnf("discovery %v: %v", pollEvent, err)

			} else {
				node.discoveryTime = pollEvent.recvTime

				controller.log.Debugf("discovery new: %v", node)

				nodes[pollEvent.String()] = node

				controller.update(nodes)
			}
		}
	}
}

func (controller *Controller) makeNode(addr *net.UDPAddr, config NodeConfig) (*Node, error) {
	var node = Node{
		log: controller.log.WithField("node", addr.String()),

		timeout: controller.config.DiscoveryTimeout,

		transport: controller.transport,
		addr:      addr,
		config:    config,
	}

	return &node, nil
}

func (controller *Controller) update(nodes map[string]*Node) {
	var discovery = Discovery{
		Nodes: make(map[string]*Node),
	}

	for name, node := range nodes {
		discovery.Nodes[name] = node
	}

	controller.discoveryState.Store(discovery)

	if controller.discoveryChan != nil {
		controller.discoveryChan <- discovery
	}
}

func (controller *Controller) Discovery() Discovery {
	if value := controller.discoveryState.Load(); value == nil {
		return Discovery{}
	} else {
		return value.(Discovery)
	}
}
