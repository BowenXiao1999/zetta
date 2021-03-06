// Copyright 2020 Zhizhesihai (Beijing) Technology Limited.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package ddl

import (
	"fmt"

	"github.com/pingcap/errors"
	"github.com/pingcap/failpoint"
	"github.com/pingcap/tidb/kv"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/tablecodec"
	"github.com/zhihu/zetta/pkg/meta"
	"github.com/zhihu/zetta/pkg/model"
	"github.com/zhihu/zetta/tablestore/infoschema"
	"github.com/zhihu/zetta/tablestore/table"
	"github.com/zhihu/zetta/tablestore/table/tables"
)

func onCreateTable(d *ddlCtx, t *meta.Meta, job *model.Job) (ver int64, _ error) {
	failpoint.Inject("mockExceedErrorLimit", func(val failpoint.Value) {
		if val.(bool) {
			failpoint.Return(ver, errors.New("mock do job error"))
		}
	})

	schemaID := job.SchemaID
	tbInfo := &model.TableMeta{}
	if err := job.DecodeArgs(tbInfo); err != nil {
		// Invalid arguments, cancel this job.
		job.State = model.JobStateCancelled
		return ver, errors.Trace(err)
	}

	tbInfo.State = model.StateNone
	err := checkTableNotExists(d, t, schemaID, tbInfo.TableName)
	if err != nil {
		if infoschema.ErrDatabaseNotExists.Equal(err) || infoschema.ErrTableExists.Equal(err) {
			job.State = model.JobStateCancelled
		}
		return ver, errors.Trace(err)
	}

	ver, err = updateSchemaVersion(t, job)
	if err != nil {
		return ver, errors.Trace(err)
	}

	switch tbInfo.State {
	case model.StateNone:
		// none -> public
		tbInfo.State = model.StatePublic
		tbInfo.UpdateTS = t.StartTS
		err = createTableOrViewWithCheck(t, job, schemaID, tbInfo)
		if err != nil {
			return ver, errors.Trace(err)
		}
		// Finish this job.
		job.FinishTableJob(model.JobStateDone, model.StatePublic, ver, tbInfo)
		//asyncNotifyEvent(d, &util.Event{Tp: model.ActionCreateTable, TableInfo: tbInfo})
		return ver, nil
	default:
		return ver, ErrInvalidTableState.GenWithStack("invalid table state %v", tbInfo.State)
	}
}

func onDropTableOrView(t *meta.Meta, job *model.Job) (ver int64, _ error) {
	tblInfo, err := checkTableExistAndCancelNonExistJob(t, job, job.SchemaID)
	if err != nil {
		return ver, errors.Trace(err)
	}

	originalState := job.SchemaState
	switch tblInfo.State {
	case model.StatePublic:
		// public -> write only
		job.SchemaState = model.StateWriteOnly
		tblInfo.State = model.StateWriteOnly
		ver, err = updateVersionAndTableInfo(t, job, tblInfo, originalState != tblInfo.State)
	case model.StateWriteOnly:
		// write only -> delete only
		job.SchemaState = model.StateDeleteOnly
		tblInfo.State = model.StateDeleteOnly
		ver, err = updateVersionAndTableInfo(t, job, tblInfo, originalState != tblInfo.State)
	case model.StateDeleteOnly:
		tblInfo.State = model.StateNone
		ver, err = updateVersionAndTableInfo(t, job, tblInfo, originalState != tblInfo.State)
		if err != nil {
			return ver, errors.Trace(err)
		}
		if err = t.DropTableOrView(job.SchemaID, job.TableID, true); err != nil {
			break
		}
		// Finish this job.
		job.FinishTableJob(model.JobStateDone, model.StateNone, ver, tblInfo)
		startKey := tablecodec.EncodeTablePrefix(job.TableID)
		job.Args = append(job.Args, startKey, getPartitionIDs(tblInfo))
	default:
		err = ErrInvalidTableState.GenWithStack("invalid table state %v", tblInfo.State)
	}

	return ver, errors.Trace(err)
}

func createTableOrViewWithCheck(t *meta.Meta, job *model.Job, schemaID int64, tbInfo *model.TableMeta) error {
	err := checkTableInfoValid(tbInfo)
	if err != nil {
		job.State = model.JobStateCancelled
		return errors.Trace(err)
	}
	return t.CreateTableOrView(schemaID, tbInfo)
}

func checkTableInfoValid(tbMeta *model.TableMeta) error {
	return nil
}

func checkTableNotExists(d *ddlCtx, t *meta.Meta, schemaID int64, tableName string) error {
	// d.infoHandle maybe nil in some test.
	if d.infoHandle == nil || !d.infoHandle.IsValid() {
		return checkTableNotExistsFromStore(t, schemaID, tableName)
	}
	// Try to use memory schema info to check first.
	currVer, err := t.GetSchemaVersion()
	if err != nil {
		return err
	}
	is := d.infoHandle.Get()
	if is.SchemaMetaVersion() == currVer {
		return checkTableNotExistsFromInfoSchema(is, schemaID, tableName)
	}

	return checkTableNotExistsFromStore(t, schemaID, tableName)
}

func checkTableNotExistsFromInfoSchema(is infoschema.InfoSchema, schemaID int64, tableName string) error {
	// Check this table's database.
	schema, ok := is.SchemaByID(schemaID)
	if !ok {
		return infoschema.ErrDatabaseNotExists.GenWithStackByArgs("")
	}
	if is.TableExists(schema.Database, tableName) {
		return infoschema.ErrTableExists.GenWithStackByArgs(tableName)
	}
	return nil
}

func checkTableNotExistsFromStore(t *meta.Meta, schemaID int64, tableName string) error {
	// Check this table's database.
	tables, err := t.ListTables(schemaID)
	if err != nil {
		if meta.ErrDBNotExists.Equal(err) {
			return infoschema.ErrDatabaseNotExists.GenWithStackByArgs("")
		}
		return errors.Trace(err)
	}

	// Check the table.
	for _, tbl := range tables {
		if tbl.TableName == tableName {
			return infoschema.ErrTableExists.GenWithStackByArgs(tbl.TableName)
		}
	}

	return nil
}

func getTableInfoAndCancelFaultJob(t *meta.Meta, job *model.Job, schemaID int64) (*model.TableMeta, error) {
	tblInfo, err := checkTableExistAndCancelNonExistJob(t, job, schemaID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if tblInfo.State != model.StatePublic {
		job.State = model.JobStateCancelled
		return nil, ErrInvalidTableState.GenWithStack("table %s is not in public, but %s", tblInfo.TableName, tblInfo.State)
	}

	return tblInfo, nil
}

func checkTableExistAndCancelNonExistJob(t *meta.Meta, job *model.Job, schemaID int64) (*model.TableMeta, error) {
	tableID := job.TableID
	// Check this table's database.
	tblInfo, err := t.GetTable(schemaID, tableID)
	if err != nil {
		if meta.ErrDBNotExists.Equal(err) {
			job.State = model.JobStateCancelled
			return nil, errors.Trace(infoschema.ErrDatabaseNotExists.GenWithStackByArgs(
				fmt.Sprintf("(Schema ID %d)", schemaID),
			))
		}
		return nil, errors.Trace(err)
	}

	// Check the table.
	if tblInfo == nil {
		job.State = model.JobStateCancelled
		return nil, errors.Trace(infoschema.ErrTableNotExists.GenWithStackByArgs(
			fmt.Sprintf("(Schema ID %d)", schemaID),
			fmt.Sprintf("(Table ID %d)", tableID),
		))
	}
	return tblInfo, nil
}

// updateVersionAndTableInfo updates the schema version and the table information.
func updateVersionAndTableInfo(t *meta.Meta, job *model.Job, tblInfo *model.TableMeta, shouldUpdateVer bool) (
	ver int64, err error) {
	if shouldUpdateVer {
		ver, err = updateSchemaVersion(t, job)
		if err != nil {
			return 0, errors.Trace(err)
		}
	}

	if tblInfo.State == model.StatePublic {
		tblInfo.UpdateTS = t.StartTS
	}
	return ver, t.UpdateTable(job.SchemaID, tblInfo)
}

func buildTableInfoWithCheck(ctx sessionctx.Context, ddl *ddl, db *model.DatabaseMeta,
	tb *model.TableMeta) (*model.TableMeta, error) {
	if ddl != nil {
		tableID, err := ddl.genGlobalIDs(1)
		if err != nil {
			return nil, errors.Trace(err)
		}
		tb.Id = tableID[0]
	}
	for i, _ := range tb.Columns {
		tb.Columns[i].Id = allocateColumnID(tb)
	}
	for i, _ := range tb.ColumnFamilies {
		tb.ColumnFamilies[i].Id = allocateColumnFamilyID(tb)
	}
	tb.ColumnFamilies = append(tb.ColumnFamilies, model.DefaultColumnFamilyMeta())
	return tb, nil
}

func addColumn(tbInfo *model.TableMeta, col *model.ColumnMeta) {
	col.Id = allocateColumnID(tbInfo)
	tbInfo.Columns = append(tbInfo.Columns, col)
}

func getPartitionIDs(table *model.TableMeta) []int64 {
	return []int64{}
}

func allocateColumnID(tblInfo *model.TableMeta) int64 {
	tblInfo.MaxColumnID++
	return tblInfo.MaxColumnID
}

func allocateColumnFamilyID(tblInfo *model.TableMeta) int64 {
	tblInfo.MaxColumnFamilyID++
	return tblInfo.MaxColumnFamilyID
}

func getTable(store kv.Storage, schemaID int64, tblInfo *model.TableMeta) (table.Table, error) {
	//alloc := autoid.NewAllocator(store, tblInfo.GetDBID(schemaID), tblInfo.IsAutoIncColUnsigned())
	tbl := tables.MakeTableFromMeta(tblInfo)
	return tbl, nil
}
