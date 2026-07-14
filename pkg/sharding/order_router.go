package sharding

import (
	"fmt"

	"tickethub/pkg/idgen"
)

type GeneOrderRouter struct {
	DBPrefix    string
	TablePrefix string
	DBCount     int
	TableCount  int
}

func (r GeneOrderRouter) RouteOrderWrites(orderNumber int64, userID int64) []Location {
	return []Location{r.RouteOrder(orderNumber, userID)}
}

func (r GeneOrderRouter) PrimaryLocations() []Location {
	if r.DBCount <= 0 {
		r.DBCount = 1
	}
	if r.TableCount <= 0 {
		r.TableCount = 1
	}
	locations := make([]Location, 0, r.DBCount*r.TableCount)
	for databaseIndex := 0; databaseIndex < r.DBCount; databaseIndex++ {
		for tableIndex := 0; tableIndex < r.TableCount; tableIndex++ {
			locations = append(locations, Location{
				Database: fmt.Sprintf("%s_%d", r.DBPrefix, databaseIndex),
				Table:    fmt.Sprintf("%s_%d", r.TablePrefix, tableIndex),
				DBIndex:  databaseIndex,
				TblIndex: tableIndex,
			})
		}
	}
	return locations
}

func NewGeneOrderRouter(dbPrefix, tablePrefix string, dbCount, tableCount int) GeneOrderRouter {
	return GeneOrderRouter{
		DBPrefix:    dbPrefix,
		TablePrefix: tablePrefix,
		DBCount:     dbCount,
		TableCount:  tableCount,
	}
}

func (r GeneOrderRouter) RouteOrder(orderNumber int64, userID int64) Location {
	if r.DBCount <= 0 {
		r.DBCount = 1
	}
	if r.TableCount <= 0 {
		r.TableCount = 1
	}
	gene := idgen.ExtractUserGene(orderNumber)
	if gene == 0 {
		gene = idgen.UserGene(userID)
	}
	tableIndex := int(gene % int64(r.TableCount))
	dbIndex := int((gene / int64(r.TableCount)) % int64(r.DBCount))
	return Location{
		Database: fmt.Sprintf("%s_%d", r.DBPrefix, dbIndex),
		Table:    fmt.Sprintf("%s_%d", r.TablePrefix, tableIndex),
		DBIndex:  dbIndex,
		TblIndex: tableIndex,
	}
}
