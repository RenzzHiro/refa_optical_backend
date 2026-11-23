package database

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/refa-optical/backend/internal/models"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed isrgrootx1.pem
var embeddedDatabaseCA []byte

type uniqueRow struct {
	ID    string `gorm:"column:id"`
	Value string `gorm:"column:value"`
	Name  string `gorm:"column:name"`
}
type foreignKeyRow struct {
	TableName      string `gorm:"column:table_name"`
	ConstraintName string `gorm:"column:constraint_name"`
}
type columnDefinition struct {
	Nullable           string `gorm:"column:is_nullable"`
	Extra              string `gorm:"column:extra"`
	DataType           string `gorm:"column:data_type"`
	CharacterMaxLength int64  `gorm:"column:character_maximum_length"`
}

func Open(dsn, caCertPath string) (*gorm.DB, error) {
	caPEM := embeddedDatabaseCA
	if caCertPath != "" {
		if configuredCA, readErr := os.ReadFile(caCertPath); readErr == nil {
			caPEM = configuredCA
		} else {
			log.Printf("database CA certificate at %q unavailable; using embedded TiDB CA: %v", caCertPath, readErr)
		}
	}
	if len(caPEM) > 0 {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("database CA certificate contains no valid PEM certificates")
		}
		tlsName := "refa-tidb"
		if err := mysqlDriver.RegisterTLSConfig(tlsName, &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}); err != nil {
			return nil, fmt.Errorf("configure database TLS: %w", err)
		}
		mysqlConfig, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("parse database DSN: %w", err)
		}
		mysqlConfig.TLSConfig = tlsName
		dsn = mysqlConfig.FormatDSN()
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn), DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	if err = dropReferencesToOrders(db); err != nil {
		return nil, fmt.Errorf("prepare order foreign keys: %w", err)
	}
	if err = db.AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.Category{}, &models.Product{}, &models.LensOption{}, &models.Order{}, &models.OrderItem{}, &models.Prescription{}, &models.Payment{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err = normalizeStringIDColumns(db); err != nil {
		return nil, fmt.Errorf("normalize UUID columns: %w", err)
	}
	if err = relaxLegacyPasswordColumn(db); err != nil {
		return nil, fmt.Errorf("migrate legacy users.password: %w", err)
	}
	if err = migrateLegacyPasswordHashes(db); err != nil {
		return nil, fmt.Errorf("migrate legacy password hashes: %w", err)
	}
	migrations := []struct {
		model                        any
		table, column, index, prefix string
		useName                      bool
	}{
		{&models.User{}, "users", "email", "idx_users_email", "legacy-user-", false},
		{&models.RefreshToken{}, "refresh_tokens", "token_hash", "idx_refresh_tokens_token_hash", "legacy-token-", false},
		{&models.Product{}, "products", "slug", "idx_products_slug_unique", "legacy-product-", true},
		{&models.Order{}, "orders", "number", "idx_orders_number", "RO-L-", false},
		{&models.Prescription{}, "prescriptions", "order_id", "idx_prescriptions_order_id", "", false},
		{&models.Payment{}, "payments", "order_id", "idx_payments_order_id", "", false},
		{&models.Payment{}, "payments", "gateway_order_id", "idx_payments_gateway_order_id", "RO-LEGACY-", false},
	}
	for _, migration := range migrations {
		if err = ensureUniqueColumn(db, migration.model, migration.table, migration.column, migration.index, migration.prefix, migration.useName); err != nil {
			return nil, fmt.Errorf("migrate %s.%s: %w", migration.table, migration.column, err)
		}
	}
	if err = restoreOrderForeignKeys(db); err != nil {
		return nil, fmt.Errorf("restore order foreign keys: %w", err)
	}
	return db, nil
}

// Older versions of the users table used a required `password` column. New
// code writes `password_hash`; allow legacy rows to remain intact while new
// inserts use the current schema.
func relaxLegacyPasswordColumn(db *gorm.DB) error {
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'password'`).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return db.Exec("ALTER TABLE `users` ALTER COLUMN `password` SET DEFAULT ''").Error
}

// Laravel stores bcrypt hashes in `password`; copy them to the column used by
// the Go auth model when that column has not been populated yet.
func migrateLegacyPasswordHashes(db *gorm.DB) error {
	var legacyColumn, hashColumn int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'password'`).Scan(&legacyColumn).Error; err != nil {
		return err
	}
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'password_hash'`).Scan(&hashColumn).Error; err != nil {
		return err
	}
	if legacyColumn == 0 || hashColumn == 0 {
		return nil
	}
	return db.Exec(`UPDATE users SET password_hash = password WHERE (password_hash IS NULL OR password_hash = '') AND password IS NOT NULL AND password LIKE '$2%'`).Error
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)
var safeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func ensureUniqueColumn(db *gorm.DB, model any, table, column, index, prefix string, useName bool) error {
	var rows []uniqueRow
	selectSQL := "id, " + column + " AS value"
	if useName {
		selectSQL += ", name"
	}
	if err := db.Table(table).Select(selectSQL).Order("id ASC").Scan(&rows).Error; err != nil {
		return err
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			suffix := strings.ReplaceAll(row.ID, "-", "")
			if len(suffix) > 12 {
				suffix = suffix[:12]
			}
			candidate := prefix + suffix
			if useName {
				base := strings.ToLower(strings.TrimSpace(row.Name))
				base = strings.Trim(slugInvalid.ReplaceAllString(base, "-"), "-")
				if base == "" {
					base = "product"
				}
				nameSuffix := strings.ReplaceAll(row.ID, "-", "")
				if len(nameSuffix) > 8 {
					nameSuffix = nameSuffix[:8]
				}
				candidate = base + "-" + nameSuffix
			}
			counter := 1
			for seen[strings.ToLower(candidate)] {
				candidate = fmt.Sprintf("%s%s-%d", prefix, suffix, counter)
				counter++
			}
			if err := db.Table(table).Where("id = ?", row.ID).Update(column, candidate).Error; err != nil {
				return err
			}
			value = candidate
			key = strings.ToLower(value)
		}
		seen[key] = true
	}
	if !db.Migrator().HasIndex(model, index) {
		statement := fmt.Sprintf("CREATE UNIQUE INDEX `%s` ON `%s` (`%s`)", index, table, column)
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func dropReferencesToOrders(db *gorm.DB) error {
	var keys []foreignKeyRow
	err := db.Raw(`SELECT DISTINCT TABLE_NAME AS table_name, CONSTRAINT_NAME AS constraint_name FROM information_schema.KEY_COLUMN_USAGE WHERE CONSTRAINT_SCHEMA = DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL AND ((REFERENCED_TABLE_NAME IN ('orders','products','users') AND TABLE_NAME IN ('order_items','prescriptions','payments','orders','refresh_tokens')) OR (TABLE_NAME IN ('order_items','prescriptions','payments','orders','refresh_tokens') AND COLUMN_NAME IN ('order_id','product_id','customer_id','user_id')))`).Scan(&keys).Error
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !safeIdentifier.MatchString(key.TableName) || !safeIdentifier.MatchString(key.ConstraintName) {
			return fmt.Errorf("unexpected foreign key identifier")
		}
		statement := fmt.Sprintf("ALTER TABLE `%s` DROP FOREIGN KEY `%s`", key.TableName, key.ConstraintName)
		if err = db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// Existing Laragon databases may have been created with numeric IDs before the
// models switched to UUID strings. Normalize both primary and related columns
// after temporarily removing their foreign keys, preserving the stored values.
func normalizeStringIDColumns(db *gorm.DB) error {
	columns := []struct{ table, column string }{
		{"users", "id"}, {"products", "id"}, {"orders", "id"},
		{"refresh_tokens", "id"}, {"prescriptions", "id"}, {"payments", "id"},
		{"refresh_tokens", "user_id"}, {"orders", "customer_id"},
		{"order_items", "order_id"}, {"order_items", "product_id"},
		{"prescriptions", "order_id"}, {"payments", "order_id"},
	}
	for _, item := range columns {
		var definition columnDefinition
		err := db.Raw(`SELECT IS_NULLABLE AS is_nullable, EXTRA AS extra, DATA_TYPE AS data_type, COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS character_maximum_length FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, item.table, item.column).Scan(&definition).Error
		if err != nil {
			return err
		}
		if definition.Nullable == "" {
			continue
		}
		// AutoMigrate already creates these UUID columns as CHAR(36). TiDB
		// rejects changing the collation of indexed CHAR columns, so leave
		// compatible columns (and their indexes) untouched.
		if strings.EqualFold(definition.DataType, "char") && definition.CharacterMaxLength == 36 {
			continue
		}
		nullability := " NULL"
		if definition.Nullable == "NO" {
			nullability = " NOT NULL"
		}
		statement := fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `%s` CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin%s", item.table, item.column, nullability)
		if err = db.Exec(statement).Error; err != nil {
			return fmt.Errorf("alter %s.%s: %w", item.table, item.column, err)
		}
	}
	return nil
}

func restoreOrderForeignKeys(db *gorm.DB) error {
	relations := []struct{ table, column, parentTable, parentColumn, constraint string }{
		{"refresh_tokens", "user_id", "users", "id", "fk_refresh_tokens_user_id"},
		{"orders", "customer_id", "users", "id", "fk_orders_customer_id"},
		{"order_items", "order_id", "orders", "id", "fk_order_items_order_id"},
		{"order_items", "product_id", "products", "id", "fk_order_items_product_id"},
		{"prescriptions", "order_id", "orders", "id", "fk_prescriptions_order_id"},
		{"payments", "order_id", "orders", "id", "fk_payments_order_id"},
	}
	for _, relation := range relations {
		if !db.Migrator().HasTable(relation.table) || !db.Migrator().HasTable(relation.parentTable) {
			continue
		}
		join := fmt.Sprintf("LEFT JOIN `%s` AS parent ON parent.`%s` = child.`%s`", relation.parentTable, relation.parentColumn, relation.column)
		where := fmt.Sprintf("parent.`%s` IS NULL", relation.parentColumn)
		var count int64
		err := db.Table(fmt.Sprintf("`%s` AS child", relation.table)).Joins(join).Where(where).Count(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			log.Printf("warning: leaving %s.%s without a foreign key because %d legacy rows do not match %s.%s", relation.table, relation.column, count, relation.parentTable, relation.parentColumn)
			continue
		}
		var existing int64
		err = db.Raw(`SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ? AND REFERENCED_TABLE_NAME = ? AND REFERENCED_COLUMN_NAME = ?`, relation.table, relation.column, relation.parentTable, relation.parentColumn).Scan(&existing).Error
		if err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE `%s` ADD CONSTRAINT `%s` FOREIGN KEY (`%s`) REFERENCES `%s` (`%s`) ON UPDATE CASCADE ON DELETE RESTRICT", relation.table, relation.constraint, relation.column, relation.parentTable, relation.parentColumn)
		if err = db.Exec(statement).Error; err != nil {
			log.Printf("warning: could not restore foreign key %s: %v", relation.constraint, err)
		}
	}
	return nil
}
