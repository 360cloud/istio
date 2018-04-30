//  Copyright 2018 Istio Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package client

import (
	"strings"
	"time"

	"istio.io/istio/galley/pkg/kube/client/serviceconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/istio/galley/pkg/change"
	"istio.io/istio/galley/pkg/kube"
	"istio.io/istio/galley/pkg/model"
	"istio.io/istio/galley/pkg/runtime"
	"istio.io/istio/pkg/log"
)

type Source struct {
	k  kube.Kube
	ch chan runtime.Event

	scAccessor *accessor
}

var _ runtime.Source = &Source{}

func NewSource(k kube.Kube, resyncPeriod time.Duration) (*Source, error) {
	s := &Source{
		k: k,
	}

	scAccessor, err := newAccessor(
		k,
		resyncPeriod,
		serviceconfig.Name,
		serviceconfig.GroupVersion,
		serviceconfig.Kind,
		serviceconfig.ListKind,
		s.process)

	if err != nil {
		return nil, err
	}
	s.scAccessor = scAccessor

	return s, nil
}

func (s *Source) Start() (chan runtime.Event, error) {
	s.ch = make(chan runtime.Event, 1024)

	s.scAccessor.start()

	return s.ch, nil
}

func (s *Source) Stop() {
	s.scAccessor.stop()
	s.ch = nil
}

func (s *Source) Get(id model.ResourceKey) (model.Resource, error) {
	parts := strings.Split(id.Name, "/")
	ns := parts[0]
	name := parts[1]
	u, err := s.scAccessor.client.Resource(&serviceconfig.APIResource, ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return model.Resource{}, err
	}

	return model.Resource{
		Key: model.ResourceKey{Kind:model.Info.ServiceConfig.Kind, Name: id.Name},
		Version: model.ResourceVersion(u.GetResourceVersion()),
		Item: serviceconfig.FromUnstructured(u),
	}, nil
}

func (s *Source) process(c *change.Info) {
	var kind runtime.EventKind
	switch c.Type {
	case change.Add:
		kind = runtime.Added
	case change.Update:
		kind = runtime.Updated
	case change.Delete:
		kind = runtime.Deleted
	case change.FullSync:
		kind = runtime.FullSync
	default:
		log.Errorf("Unknown change kind: %v", c.Type)
	}

	rid := model.ResourceKey{ Kind: model.ResourceKind(serviceconfig.Kind), Name: c.Name }

	e := runtime.Event{
		Id:   rid,
		Version: model.ResourceVersion(c.Version),
		Kind: kind,
	}

	log.Debugf("Dispatching source event: %v", e)
	s.ch <- e
}