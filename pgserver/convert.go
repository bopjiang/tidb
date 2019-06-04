package pgserver

// Copyright (c) 2018 Jiang Jia
// Apache License 2.0

import "github.com/pingcap/tidb/pgserver/oid"

// from github.com/go-sql-driver/mysql/const.go
// https://dev.mysql.com/doc/internals/en/com-query-response.html#packet-Protocol::ColumnType
const (
	fieldTypeDecimal byte = iota
	fieldTypeTiny
	fieldTypeShort
	fieldTypeLong
	fieldTypeFloat
	fieldTypeDouble
	fieldTypeNULL
	fieldTypeTimestamp
	fieldTypeLongLong
	fieldTypeInt24
	fieldTypeDate
	fieldTypeTime
	fieldTypeDateTime
	fieldTypeYear
	fieldTypeNewDate
	fieldTypeVarChar
	fieldTypeBit
)

const (
	fieldTypeJSON byte = iota + 0xf5
	fieldTypeNewDecimal
	fieldTypeEnum
	fieldTypeSet
	fieldTypeTinyBLOB
	fieldTypeMediumBLOB
	fieldTypeLongBLOB
	fieldTypeBLOB
	fieldTypeVarString
	fieldTypeString
	fieldTypeGeometry
)

// convertMysqlTypeToOid returns Oid and data type size. Note that negative values denote variable-width types.
func convertMysqlTypeToOid(typ uint8) (oid.Oid, int) {
	switch typ {
	case fieldTypeLong:
		return oid.T_int4, 4
	case fieldTypeBLOB:
		return oid.T__text, -1
	case fieldTypeVarChar:
		return oid.T__text, -1
	}
	return oid.T_unknown, -1
}
