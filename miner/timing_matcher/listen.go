/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package timing_matcher

import (
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/types"
	"math/big"
	"sync"
	"time"
)

//func (matcher *TimingMatcher) listenNewBlock() {
//	newBlockChan := make(chan *types.BlockEvent)
//
//	go func() {
//		for {
//			select {
//			case blockEvent := <-newBlockChan:
//				if nil != blockEvent {
//					nextBlockNumber := new(big.Int).Add(matcher.duration, matcher.lastRoundNumber)
//					if nextBlockNumber.Cmp(blockEvent.BlockNumber) <= 0 {
//						// debug use only
//						// log.Debugf("miner starts a new match round")
//						matcher.lastRoundNumber = blockEvent.BlockNumber
//						matcher.rounds.appendNewRoundState(matcher.lastRoundNumber)
//						var wg sync.WaitGroup
//						for _, market := range matcher.markets {
//							wg.Add(1)
//							go func(m *Market) {
//								defer func() {
//									wg.Add(-1)
//								}()
//								m.match()
//							}(market)
//						}
//						wg.Wait()
//					}
//				}
//			}
//		}
//	}()
//
//	watcher := &eventemitter.Watcher{
//		Concurrent: false,
//		Handle: func(eventData eventemitter.EventData) error {
//			blockEvent := eventData.(*types.BlockEvent)
//			newBlockChan <- blockEvent
//			return nil
//		},
//	}
//	eventemitter.On(eventemitter.Block_New, watcher)
//	matcher.stopFuncs = append(matcher.stopFuncs, func() {
//		close(newBlockChan)
//		eventemitter.Un(eventemitter.Block_New, watcher)
//	})
//
//}

func (matcher *TimingMatcher) listenTimingRound() {
	stopChan := make(chan bool)

	matchFunc := func() {
		//if ethaccessor.Synced() {
		matcher.lastRoundNumber = big.NewInt(time.Now().UnixNano() / 1e6)
		//matcher.rounds.appendNewRoundState(matcher.lastRoundNumber)
		var wg sync.WaitGroup
		for _, market := range matcher.markets {
			wg.Add(1)
			go func(m *Market) {
				defer func() {
					wg.Add(-1)
				}()
				m.match()
			}(market)
		}
		wg.Wait()
		//}
	}
	go func() {
		matchFunc()
		for {
			select {
			case <-time.After(time.Duration(matcher.duration.Int64()) * time.Millisecond):
				matchFunc()
			case <-stopChan:
				return
			}
		}
	}()

	matcher.stopFuncs = append(matcher.stopFuncs, func() {
		stopChan <- true
		close(stopChan)
	})
}

func (matcher *TimingMatcher) listenSubmitEvent() {
	submitEventChan := make(chan *types.RingSubmitResultEvent)
	go func() {
		for {
			select {
			case minedEvent := <-submitEventChan:
				if minedEvent.Status == types.TX_STATUS_FAILED || minedEvent.Status == types.TX_STATUS_SUCCESS || minedEvent.Status == types.TX_STATUS_UNKNOWN {
					log.Debugf("received mined event, this round the related cache will be removed, ringhash:%s, status:%d", minedEvent.RingHash.Hex(), minedEvent.Status)
					//matcher.rounds.RemoveMinedRing(minedEvent.RingHash)
					if err := RemoveMinedRing(minedEvent.RingHash); nil != err {
						log.Errorf("err:%s", err.Error())
					}
				}
			}
		}
	}()

	//submitWatcher := &eventemitter.Watcher{
	//	Concurrent: false,
	//	Handle: func(eventData eventemitter.EventData) error {
	//		minedEvent := eventData.(*types.RingMinedEvent)
	//		submitEventChan <- minedEvent.Ringhash
	//		return nil
	//	},
	//}

	submitResultWatcher := &eventemitter.Watcher{
		Concurrent: false,
		Handle: func(eventData eventemitter.EventData) error {
			minedEvent := eventData.(*types.RingSubmitResultEvent)
			submitEventChan <- minedEvent
			return nil
		},
	}

	//eventemitter.On(eventemitter.OrderManagerExtractorRingMined, submitWatcher)
	eventemitter.On(eventemitter.Miner_RingSubmitResult, submitResultWatcher)
	matcher.stopFuncs = append(matcher.stopFuncs, func() {
		//eventemitter.Un(eventemitter.OrderManagerExtractorRingMined, submitWatcher)
		eventemitter.Un(eventemitter.Miner_RingSubmitResult, submitResultWatcher)
		close(submitEventChan)
	})
}
