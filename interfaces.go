package main

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

var classes = []string{"Damper", "VAV", "Sensor", "Command", "Setpoint", "Status"}

type Damper struct {
	Name     string
	Position string
}

type Writeable interface {
	GetName() string
	AddReading(name, class string, time int64, value float64)
}

//    query := fmt.Sprintf(`SELECT ?name ?class ?equip ?equipclass WHERE {
//        ?name bf:uuid "%s" .
//        ?name rdf:type ?class .
//        {
//            ?name bf:isPointOf ?equip .
//            OR
//            ?name bf:isPartOf ?equip .
//        }
//        ?equip rdf:type ?equipclass .
//    };`, msg.UUID)

func (s *server) forward(uuid string, data [][]json.Number, baseuri string) error {

	query := fmt.Sprintf(`SELECT ?name ?class ?equip ?equipclass WHERE {
            ?name bf:uuid "%s" .
            ?name rdf:type ?class .
            {
                ?name bf:isPointOf ?equip .
                OR
                ?name bf:isPartOf ?equip .
            }
            ?equip rdf:type ?equipclass .
        };`, uuid)
	res, err := s.hod.DoQuery(query, nil)
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return errors.New("No results")
	}

	row := res.Rows[0]
	point_class := row["?class"].Value
	equipment_class := row["?equipclass"].Value
	equipment_name := row["?equip"].Value
	log.Debug(point_class, equipment_class)

	// the publish/interface URI is constructed as
	// baseuri + s.bms + equipment name + i.equipment type + signal + info

	var generic_point_class, generic_equip_class string

	for _, superclass := range classes {
		if f, err := s.isSubclassOf(point_class, superclass); err != nil {
			return err
		} else if f {
			log.Debugf("%s is subclass of %s", point_class, superclass)
			generic_point_class = superclass
			break
		}
	}

	for _, superclass := range classes {
		if f, err := s.isSubclassOf(equipment_class, superclass); err != nil {
			return err
		} else if f {
			log.Debugf("%s is subclass of %s", equipment_class, superclass)
			generic_equip_class = superclass
			break
		}
	}

	uri := fmt.Sprintf("%s/s.bms/%s/i.%s/signal/info", baseuri, equipment_name, generic_equip_class)
	err = s.publish(SmapParams{
		Data:           data,
		URI:            uri,
		Name:           row["?name"].Value,
		Class:          generic_point_class,
		Equipment:      equipment_name,
		EquipmentClass: generic_equip_class,
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *server) isSubclassOf(subclass, superclass string) (bool, error) {

	query := fmt.Sprintf(`SELECT ?class WHERE {
            ?class rdfs:subClassOf* brick:%s .
        };`, superclass)
	res, err := s.hod.DoQuery(query, nil)
	if err != nil {
		return false, err
	}
	if len(res.Rows) == 0 {
		return false, errors.New("No results")
	}

	for _, row := range res.Rows {
		if row["?class"].Value == subclass {
			return true, nil
		}
	}

	return false, nil

}
