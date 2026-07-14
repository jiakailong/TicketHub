package sharding

import "fmt"

type ModRouter struct {
	DBPrefix    string
	TablePrefix string
	DBCount     int
	TableCount  int
}

func (r ModRouter) Route(id int64) Location {
	if r.DBCount <= 0 {
		r.DBCount = 1
	}
	if r.TableCount <= 0 {
		r.TableCount = 1
	}
	if id < 0 {
		id = -id
	}
	dbIndex := int(id % int64(r.DBCount))
	tableIndex := int((id / int64(r.DBCount)) % int64(r.TableCount))
	return Location{
		Database: fmt.Sprintf("%s_%d", r.DBPrefix, dbIndex),
		Table:    fmt.Sprintf("%s_%d", r.TablePrefix, tableIndex),
		DBIndex:  dbIndex,
		TblIndex: tableIndex,
	}
}
