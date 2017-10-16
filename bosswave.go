package main

import (
	"encoding/json"

	bw2 "gopkg.in/immesys/bw2bind.v5"
)

type SmapParams struct {
	Data           [][]json.Number
	URI            string
	Name           string
	Class          string
	Equipment      string
	EquipmentClass string
}

type DataMessage struct {
	Time                                   int64
	Value                                  float64
	Name, Class, Equipment, EquipmentClass string
}

func (s *server) publish(params SmapParams) error {
	for _, datum := range params.Data {
		var msg = DataMessage{
			Name:           params.Name,
			Class:          params.Class,
			Equipment:      params.Equipment,
			EquipmentClass: params.EquipmentClass,
		}
		if time, err := datum[0].Int64(); err != nil {
			return err
		} else {
			msg.Time = time
		}

		if value, err := datum[1].Float64(); err != nil {
			return err
		} else {
			msg.Value = value
		}

		po, err := bw2.CreateMsgPackPayloadObject(bw2.FromDotForm("2.0.0.0"), msg)
		if err != nil {
			return err
		}
		if err := s.bw2.Publish(&bw2.PublishParams{
			URI:            params.URI,
			PayloadObjects: []bw2.PayloadObject{po},
		}); err != nil {
			return err
		}
	}

	return nil
}
