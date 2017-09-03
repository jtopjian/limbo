package db

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/lxc/lxd/lxd/db/schema"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/logger"
)

/* Database updates are one-time actions that are needed to move an
   existing database from one version of the schema to the next.

   Those updates are applied at startup time before anything else in LXD
   is initialized. This means that they should be entirely
   self-contained and not touch anything but the database.

   Calling LXD functions isn't allowed as such functions may themselves
   depend on a newer DB schema and so would fail when upgrading a very old
   version of LXD.

   DO NOT USE this mechanism for one-time actions which do not involve
   changes to the database schema. Use patches instead (see lxd/patches.go).

   REMEMBER to run "make update-schema" after you add a new update function to
   this slice. That will refresh the schema declaration in lxd/db/schema.go and
   include the effect of applying your patch as well.

   Only append to the updates list, never remove entries and never re-order them.
*/

var updates = map[int]schema.Update{
	1:  updateFromV0,
	2:  updateFromV1,
	3:  updateFromV2,
	4:  updateFromV3,
	5:  updateFromV4,
	6:  updateFromV5,
	7:  updateFromV6,
	8:  updateFromV7,
	9:  updateFromV8,
	10: updateFromV9,
	11: updateFromV10,
	12: updateFromV11,
	13: updateFromV12,
	14: updateFromV13,
	15: updateFromV14,
	16: updateFromV15,
	17: updateFromV16,
	18: updateFromV17,
	19: updateFromV18,
	20: updateFromV19,
	21: updateFromV20,
	22: updateFromV21,
	23: updateFromV22,
	24: updateFromV23,
	25: updateFromV24,
	26: updateFromV25,
	27: updateFromV26,
	28: updateFromV27,
	29: updateFromV28,
	30: updateFromV29,
	31: updateFromV30,
	32: updateFromV31,
	33: updateFromV32,
	34: updateFromV33,
	35: updateFromV34,
	36: updateFromV35,
}

// LegacyPatch is a "database" update that performs non-database work. They
// are needed for historical reasons, since there was a time were db updates
// could do non-db work and depend on functionality external to the db
// package. See UpdatesApplyAll below.
type LegacyPatch struct {
	NeedsDB bool         // Whether the patch does any DB-related work
	Hook    func() error // The actual patch logic
}

// UpdatesApplyAll applies all possible database patches. If "doBackup" is
// true, the sqlite file will be backed up before any update is applied. The
// legacyPatches parameter is used by the Daemon as a mean to apply the legacy
// V10, V11, V15, V29 and V30 non-db updates during the database upgrade
// sequence, to avoid any change in semantics wrt the old logic (see PR #3322).
func UpdatesApplyAll(db *sql.DB, doBackup bool, legacyPatches map[int]*LegacyPatch) error {
	backup := false

	schema := schema.NewFromMap(updates)
	schema.Hook(func(version int, tx *sql.Tx) error {
		if doBackup && !backup {
			logger.Infof("Updating the LXD database schema. Backup made as \"lxd.db.bak\"")
			err := shared.FileCopy(shared.VarPath("lxd.db"), shared.VarPath("lxd.db.bak"))
			if err != nil {
				return err
			}

			backup = true
		}
		logger.Debugf("Updating DB schema from %d to %d", version, version+1)

		legacyPatch, ok := legacyPatches[version]
		if ok {
			// FIXME We need to commit the transaction before the
			// hook and then open it again afterwards because this
			// legacy patch pokes with the database and would fail
			// with a lock error otherwise.
			if legacyPatch.NeedsDB {
				_, err := tx.Exec("COMMIT")
				if err != nil {
					return err
				}
			}
			err := legacyPatch.Hook()
			if err != nil {
				return err
			}
			if legacyPatch.NeedsDB {
				_, err = tx.Exec("BEGIN")
			}

			return err
		}
		return nil
	})
	return schema.Ensure(db)
}

// UpdateSchemaDotGo rewrites the 'schema.go' source file in this package to
// match the current schema updates.
//
// The schema.go file contains a "flattened" render of all schema updates
// defined in this file, and it's used to initialize brand new databases.
func UpdateSchemaDotGo() error {
	// Apply all the updates that we have on a pristine database and dump
	// the resulting schema.
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("failed to open schema.go for writing: %v", err)
	}

	schema := schema.NewFromMap(updates)

	err = schema.Ensure(db)
	if err != nil {
		return err
	}

	dump, err := schema.Dump(db)
	if err != nil {
		return err
	}

	// Passing 1 to runtime.Caller identifies the caller of runtime.Caller,
	// that means us.
	_, filename, _, _ := runtime.Caller(0)

	file, err := os.Create(path.Join(path.Dir(filename), "schema.go"))
	if err != nil {
		return fmt.Errorf("failed to open schema.go for writing: %v", err)
	}

	_, err = file.Write([]byte(fmt.Sprintf(schemaDotGo, dump)))
	if err != nil {
		return fmt.Errorf("failed to write to schema.go: %v", err)
	}

	return nil
}

// Template for schema.go (can't use backticks since we need to use backticks
// inside the template itself).
const schemaDotGo = "package db\n\n" +
	"// DO NOT EDIT BY HAND\n" +
	"//\n" +
	"// This code was generated by the UpdateSchemaDotGo function. If you need to\n" +
	"// modify the database schema, please add a new schema update to update.go\n" +
	"// and the run 'make update-schema'.\n" +
	"const CURRENT_SCHEMA = `\n" +
	"%s`\n"

// Schema updates begin here
func updateFromV35(tx *sql.Tx) error {
	stmts := `
CREATE TABLE tmp (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    image_id INTEGER NOT NULL,
    description TEXT,
    FOREIGN KEY (image_id) REFERENCES images (id) ON DELETE CASCADE,
    UNIQUE (name)
);
INSERT INTO tmp (id, name, image_id, description)
    SELECT id, name, image_id, description
    FROM images_aliases;
DROP TABLE images_aliases;
ALTER TABLE tmp RENAME TO images_aliases;

ALTER TABLE networks ADD COLUMN description TEXT;
ALTER TABLE storage_pools ADD COLUMN description TEXT;
ALTER TABLE storage_volumes ADD COLUMN description TEXT;
ALTER TABLE containers ADD COLUMN description TEXT;
`
	_, err := tx.Exec(stmts)
	return err
}

func updateFromV34(tx *sql.Tx) error {
	stmt := `
CREATE TABLE IF NOT EXISTS storage_pools (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    driver VARCHAR(255) NOT NULL,
    UNIQUE (name)
);
CREATE TABLE IF NOT EXISTS storage_pools_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    storage_pool_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (storage_pool_id, key),
    FOREIGN KEY (storage_pool_id) REFERENCES storage_pools (id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS storage_volumes (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    storage_pool_id INTEGER NOT NULL,
    type INTEGER NOT NULL,
    UNIQUE (storage_pool_id, name, type),
    FOREIGN KEY (storage_pool_id) REFERENCES storage_pools (id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS storage_volumes_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    storage_volume_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (storage_volume_id, key),
    FOREIGN KEY (storage_volume_id) REFERENCES storage_volumes (id) ON DELETE CASCADE
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV33(tx *sql.Tx) error {
	stmt := `
CREATE TABLE IF NOT EXISTS networks (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    UNIQUE (name)
);
CREATE TABLE IF NOT EXISTS networks_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    network_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (network_id, key),
    FOREIGN KEY (network_id) REFERENCES networks (id) ON DELETE CASCADE
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV32(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE containers ADD COLUMN last_use_date DATETIME;")
	return err
}

func updateFromV31(tx *sql.Tx) error {
	stmt := `
CREATE TABLE IF NOT EXISTS patches (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    applied_at DATETIME NOT NULL,
    UNIQUE (name)
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV30(tx *sql.Tx) error {
	// NOTE: this database update contained daemon-level logic which
	//       was been moved to patchUpdateFromV15 in patches.go.
	return nil
}

func updateFromV29(tx *sql.Tx) error {
	if shared.PathExists(shared.VarPath("zfs.img")) {
		err := os.Chmod(shared.VarPath("zfs.img"), 0600)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateFromV28(tx *sql.Tx) error {
	stmt := `
INSERT INTO profiles_devices (profile_id, name, type) SELECT id, "aadisable", 2 FROM profiles WHERE name="docker";
INSERT INTO profiles_devices_config (profile_device_id, key, value) SELECT profiles_devices.id, "source", "/dev/null" FROM profiles_devices LEFT JOIN profiles WHERE profiles_devices.profile_id = profiles.id AND profiles.name = "docker" AND profiles_devices.name = "aadisable";
INSERT INTO profiles_devices_config (profile_device_id, key, value) SELECT profiles_devices.id, "path", "/sys/module/apparmor/parameters/enabled" FROM profiles_devices LEFT JOIN profiles WHERE profiles_devices.profile_id = profiles.id AND profiles.name = "docker" AND profiles_devices.name = "aadisable";`
	tx.Exec(stmt)

	return nil
}

func updateFromV27(tx *sql.Tx) error {
	_, err := tx.Exec("UPDATE profiles_devices SET type=3 WHERE type='unix-char';")
	return err
}

func updateFromV26(tx *sql.Tx) error {
	stmt := `
ALTER TABLE images ADD COLUMN auto_update INTEGER NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS images_source (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    image_id INTEGER NOT NULL,
    server TEXT NOT NULL,
    protocol INTEGER NOT NULL,
    certificate TEXT NOT NULL,
    alias VARCHAR(255) NOT NULL,
    FOREIGN KEY (image_id) REFERENCES images (id) ON DELETE CASCADE
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV25(tx *sql.Tx) error {
	stmt := `
INSERT INTO profiles (name, description) VALUES ("docker", "Profile supporting docker in containers");
INSERT INTO profiles_config (profile_id, key, value) SELECT id, "security.nesting", "true" FROM profiles WHERE name="docker";
INSERT INTO profiles_config (profile_id, key, value) SELECT id, "linux.kernel_modules", "overlay, nf_nat" FROM profiles WHERE name="docker";
INSERT INTO profiles_devices (profile_id, name, type) SELECT id, "fuse", "unix-char" FROM profiles WHERE name="docker";
INSERT INTO profiles_devices_config (profile_device_id, key, value) SELECT profiles_devices.id, "path", "/dev/fuse" FROM profiles_devices LEFT JOIN profiles WHERE profiles_devices.profile_id = profiles.id AND profiles.name = "docker";`
	tx.Exec(stmt)

	return nil
}

func updateFromV24(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE containers ADD COLUMN stateful INTEGER NOT NULL DEFAULT 0;")
	return err
}

func updateFromV23(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE profiles ADD COLUMN description TEXT;")
	return err
}

func updateFromV22(tx *sql.Tx) error {
	stmt := `
DELETE FROM containers_devices_config WHERE key='type';
DELETE FROM profiles_devices_config WHERE key='type';`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV21(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE containers ADD COLUMN creation_date DATETIME NOT NULL DEFAULT 0;")
	return err
}

func updateFromV20(tx *sql.Tx) error {
	stmt := `
UPDATE containers_devices SET name='__lxd_upgrade_root' WHERE name='root';
UPDATE profiles_devices SET name='__lxd_upgrade_root' WHERE name='root';

INSERT INTO containers_devices (container_id, name, type) SELECT id, "root", 2 FROM containers;
INSERT INTO containers_devices_config (container_device_id, key, value) SELECT id, "path", "/" FROM containers_devices WHERE name='root';`
	_, err := tx.Exec(stmt)

	return err
}

func updateFromV19(tx *sql.Tx) error {
	stmt := `
DELETE FROM containers_config WHERE container_id NOT IN (SELECT id FROM containers);
DELETE FROM containers_devices_config WHERE container_device_id NOT IN (SELECT id FROM containers_devices WHERE container_id IN (SELECT id FROM containers));
DELETE FROM containers_devices WHERE container_id NOT IN (SELECT id FROM containers);
DELETE FROM containers_profiles WHERE container_id NOT IN (SELECT id FROM containers);
DELETE FROM images_aliases WHERE image_id NOT IN (SELECT id FROM images);
DELETE FROM images_properties WHERE image_id NOT IN (SELECT id FROM images);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV18(tx *sql.Tx) error {
	var id int
	var value string

	// Update container config
	rows, err := QueryScan(tx, "SELECT id, value FROM containers_config WHERE key='limits.memory'", nil, []interface{}{id, value})
	if err != nil {
		return err
	}

	for _, row := range rows {
		id = row[0].(int)
		value = row[1].(string)

		// If already an integer, don't touch
		_, err := strconv.Atoi(value)
		if err == nil {
			continue
		}

		// Generate the new value
		value = strings.ToUpper(value)
		value += "B"

		// Deal with completely broken values
		_, err = shared.ParseByteSizeString(value)
		if err != nil {
			logger.Debugf("Invalid container memory limit, id=%d value=%s, removing.", id, value)
			_, err = tx.Exec("DELETE FROM containers_config WHERE id=?;", id)
			if err != nil {
				return err
			}
		}

		// Set the new value
		_, err = tx.Exec("UPDATE containers_config SET value=? WHERE id=?", value, id)
		if err != nil {
			return err
		}
	}

	// Update profiles config
	rows, err = QueryScan(tx, "SELECT id, value FROM profiles_config WHERE key='limits.memory'", nil, []interface{}{id, value})
	if err != nil {
		return err
	}

	for _, row := range rows {
		id = row[0].(int)
		value = row[1].(string)

		// If already an integer, don't touch
		_, err := strconv.Atoi(value)
		if err == nil {
			continue
		}

		// Generate the new value
		value = strings.ToUpper(value)
		value += "B"

		// Deal with completely broken values
		_, err = shared.ParseByteSizeString(value)
		if err != nil {
			logger.Debugf("Invalid profile memory limit, id=%d value=%s, removing.", id, value)
			_, err = tx.Exec("DELETE FROM profiles_config WHERE id=?;", id)
			if err != nil {
				return err
			}
		}

		// Set the new value
		_, err = tx.Exec("UPDATE profiles_config SET value=? WHERE id=?", value, id)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateFromV17(tx *sql.Tx) error {
	stmt := `
DELETE FROM profiles_config WHERE key LIKE 'volatile.%';
UPDATE containers_config SET key='limits.cpu' WHERE key='limits.cpus';
UPDATE profiles_config SET key='limits.cpu' WHERE key='limits.cpus';`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV16(tx *sql.Tx) error {
	stmt := `
UPDATE config SET key='storage.lvm_vg_name' WHERE key = 'core.lvm_vg_name';
UPDATE config SET key='storage.lvm_thinpool_name' WHERE key = 'core.lvm_thinpool_name';`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV15(tx *sql.Tx) error {
	// NOTE: this database update contained daemon-level logic which
	//       was been moved to patchUpdateFromV15 in patches.go.
	return nil
}

func updateFromV14(tx *sql.Tx) error {
	stmt := `
PRAGMA foreign_keys=OFF; -- So that integrity doesn't get in the way for now

DELETE FROM containers_config WHERE key="volatile.last_state.power";

INSERT INTO containers_config (container_id, key, value)
    SELECT id, "volatile.last_state.power", "RUNNING"
    FROM containers WHERE power_state=1;

INSERT INTO containers_config (container_id, key, value)
    SELECT id, "volatile.last_state.power", "STOPPED"
    FROM containers WHERE power_state != 1;

CREATE TABLE tmp (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    architecture INTEGER NOT NULL,
    type INTEGER NOT NULL,
    ephemeral INTEGER NOT NULL DEFAULT 0,
    UNIQUE (name)
);

INSERT INTO tmp SELECT id, name, architecture, type, ephemeral FROM containers;

DROP TABLE containers;
ALTER TABLE tmp RENAME TO containers;

PRAGMA foreign_keys=ON; -- Make sure we turn integrity checks back on.`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV13(tx *sql.Tx) error {
	stmt := `
UPDATE containers_config SET key='volatile.base_image' WHERE key = 'volatile.baseImage';`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV12(tx *sql.Tx) error {
	stmt := `
ALTER TABLE images ADD COLUMN cached INTEGER NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN last_use_date DATETIME;`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV11(tx *sql.Tx) error {
	// NOTE: this database update contained daemon-level logic which
	//       was been moved to patchUpdateFromV15 in patches.go.
	return nil
}

func updateFromV10(tx *sql.Tx) error {
	// NOTE: this database update contained daemon-level logic which
	//       was been moved to patchUpdateFromV10 in patches.go.
	return nil
}

func updateFromV9(tx *sql.Tx) error {
	stmt := `
CREATE TABLE tmp (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(255) NOT NULL default "none",
    FOREIGN KEY (container_id) REFERENCES containers (id) ON DELETE CASCADE,
    UNIQUE (container_id, name)
);

INSERT INTO tmp SELECT * FROM containers_devices;

UPDATE containers_devices SET type=0 WHERE id IN (SELECT id FROM tmp WHERE type="none");
UPDATE containers_devices SET type=1 WHERE id IN (SELECT id FROM tmp WHERE type="nic");
UPDATE containers_devices SET type=2 WHERE id IN (SELECT id FROM tmp WHERE type="disk");
UPDATE containers_devices SET type=3 WHERE id IN (SELECT id FROM tmp WHERE type="unix-char");
UPDATE containers_devices SET type=4 WHERE id IN (SELECT id FROM tmp WHERE type="unix-block");

DROP TABLE tmp;

CREATE TABLE tmp (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    profile_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(255) NOT NULL default "none",
    FOREIGN KEY (profile_id) REFERENCES profiles (id) ON DELETE CASCADE,
    UNIQUE (profile_id, name)
);

INSERT INTO tmp SELECT * FROM profiles_devices;

UPDATE profiles_devices SET type=0 WHERE id IN (SELECT id FROM tmp WHERE type="none");
UPDATE profiles_devices SET type=1 WHERE id IN (SELECT id FROM tmp WHERE type="nic");
UPDATE profiles_devices SET type=2 WHERE id IN (SELECT id FROM tmp WHERE type="disk");
UPDATE profiles_devices SET type=3 WHERE id IN (SELECT id FROM tmp WHERE type="unix-char");
UPDATE profiles_devices SET type=4 WHERE id IN (SELECT id FROM tmp WHERE type="unix-block");

DROP TABLE tmp;`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV8(tx *sql.Tx) error {
	stmt := `
UPDATE certificates SET fingerprint = replace(fingerprint, " ", "");`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV7(tx *sql.Tx) error {
	stmt := `
UPDATE config SET key='core.trust_password' WHERE key IN ('password', 'trust_password', 'trust-password', 'core.trust-password');
DELETE FROM config WHERE key != 'core.trust_password';`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV6(tx *sql.Tx) error {
	// This update recreates the schemas that need an ON DELETE CASCADE foreign
	// key.
	stmt := `
PRAGMA foreign_keys=OFF; -- So that integrity doesn't get in the way for now

CREATE TEMP TABLE tmp AS SELECT * FROM containers_config;
DROP TABLE containers_config;
CREATE TABLE IF NOT EXISTS containers_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (container_id) REFERENCES containers (id) ON DELETE CASCADE,
    UNIQUE (container_id, key)
);
INSERT INTO containers_config SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM containers_devices;
DROP TABLE containers_devices;
CREATE TABLE IF NOT EXISTS containers_devices (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type INTEGER NOT NULL default 0,
    FOREIGN KEY (container_id) REFERENCES containers (id) ON DELETE CASCADE,
    UNIQUE (container_id, name)
);
INSERT INTO containers_devices SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM containers_devices_config;
DROP TABLE containers_devices_config;
CREATE TABLE IF NOT EXISTS containers_devices_config (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_device_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (container_device_id) REFERENCES containers_devices (id) ON DELETE CASCADE,
    UNIQUE (container_device_id, key)
);
INSERT INTO containers_devices_config SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM containers_profiles;
DROP TABLE containers_profiles;
CREATE TABLE IF NOT EXISTS containers_profiles (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    profile_id INTEGER NOT NULL,
    apply_order INTEGER NOT NULL default 0,
    UNIQUE (container_id, profile_id),
    FOREIGN KEY (container_id) REFERENCES containers(id) ON DELETE CASCADE,
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);
INSERT INTO containers_profiles SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM images_aliases;
DROP TABLE images_aliases;
CREATE TABLE IF NOT EXISTS images_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    image_id INTEGER NOT NULL,
    description VARCHAR(255),
    FOREIGN KEY (image_id) REFERENCES images (id) ON DELETE CASCADE,
    UNIQUE (name)
);
INSERT INTO images_aliases SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM images_properties;
DROP TABLE images_properties;
CREATE TABLE IF NOT EXISTS images_properties (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    image_id INTEGER NOT NULL,
    type INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (image_id) REFERENCES images (id) ON DELETE CASCADE
);
INSERT INTO images_properties SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM profiles_config;
DROP TABLE profiles_config;
CREATE TABLE IF NOT EXISTS profiles_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value VARCHAR(255),
    UNIQUE (profile_id, key),
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);
INSERT INTO profiles_config SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM profiles_devices;
DROP TABLE profiles_devices;
CREATE TABLE IF NOT EXISTS profiles_devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type INTEGER NOT NULL default 0,
    UNIQUE (profile_id, name),
    FOREIGN KEY (profile_id) REFERENCES profiles (id) ON DELETE CASCADE
);
INSERT INTO profiles_devices SELECT * FROM tmp;
DROP TABLE tmp;

CREATE TEMP TABLE tmp AS SELECT * FROM profiles_devices_config;
DROP TABLE profiles_devices_config;
CREATE TABLE IF NOT EXISTS profiles_devices_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_device_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (profile_device_id, key),
    FOREIGN KEY (profile_device_id) REFERENCES profiles_devices (id) ON DELETE CASCADE
);
INSERT INTO profiles_devices_config SELECT * FROM tmp;
DROP TABLE tmp;

PRAGMA foreign_keys=ON; -- Make sure we turn integrity checks back on.`
	_, err := tx.Exec(stmt)
	if err != nil {
		return err
	}

	// Get the rows with broken foreign keys an nuke them
	rows, err := tx.Query("PRAGMA foreign_key_check;")
	if err != nil {
		return err
	}

	var tablestodelete []string
	var rowidtodelete []int

	defer rows.Close()
	for rows.Next() {
		var tablename string
		var rowid int
		var targetname string
		var keynumber int

		rows.Scan(&tablename, &rowid, &targetname, &keynumber)
		tablestodelete = append(tablestodelete, tablename)
		rowidtodelete = append(rowidtodelete, rowid)
	}
	rows.Close()

	for i := range tablestodelete {
		_, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE rowid = %d;", tablestodelete[i], rowidtodelete[i]))
		if err != nil {
			return err
		}
	}

	return err
}

func updateFromV5(tx *sql.Tx) error {
	stmt := `
ALTER TABLE containers ADD COLUMN power_state INTEGER NOT NULL DEFAULT 0;
ALTER TABLE containers ADD COLUMN ephemeral INTEGER NOT NULL DEFAULT 0;`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV4(tx *sql.Tx) error {
	stmt := `
CREATE TABLE IF NOT EXISTS config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (key)
);`

	_, err := tx.Exec(stmt)
	if err != nil {
		return err
	}

	passfname := shared.VarPath("adminpwd")
	passOut, err := os.Open(passfname)
	oldPassword := ""
	if err == nil {
		defer passOut.Close()
		buff := make([]byte, 96)
		_, err = passOut.Read(buff)
		if err != nil {
			return err
		}

		oldPassword = hex.EncodeToString(buff)
		stmt := `INSERT INTO config (key, value) VALUES ("core.trust_password", ?);`

		_, err := tx.Exec(stmt, oldPassword)
		if err != nil {
			return err
		}

		return os.Remove(passfname)
	}

	return nil
}

func updateFromV3(tx *sql.Tx) error {
	// Attempt to create a default profile (but don't fail if already there)
	tx.Exec("INSERT INTO profiles (name) VALUES (\"default\");")

	return nil
}

func updateFromV2(tx *sql.Tx) error {
	stmt := `
CREATE TABLE IF NOT EXISTS containers_devices (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type INTEGER NOT NULL default 0,
    FOREIGN KEY (container_id) REFERENCES containers (id) ON DELETE CASCADE,
    UNIQUE (container_id, name)
);
CREATE TABLE IF NOT EXISTS containers_devices_config (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_device_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (container_device_id) REFERENCES containers_devices (id),
    UNIQUE (container_device_id, key)
);
CREATE TABLE IF NOT EXISTS containers_profiles (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    profile_id INTEGER NOT NULL,
    apply_order INTEGER NOT NULL default 0,
    UNIQUE (container_id, profile_id),
    FOREIGN KEY (container_id) REFERENCES containers(id) ON DELETE CASCADE,
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    UNIQUE (name)
);
CREATE TABLE IF NOT EXISTS profiles_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value VARCHAR(255),
    UNIQUE (profile_id, key),
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS profiles_devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    type INTEGER NOT NULL default 0,
    UNIQUE (profile_id, name),
    FOREIGN KEY (profile_id) REFERENCES profiles (id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS profiles_devices_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    profile_device_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    UNIQUE (profile_device_id, key),
    FOREIGN KEY (profile_device_id) REFERENCES profiles_devices (id)
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV1(tx *sql.Tx) error {
	// v1..v2 adds images aliases
	stmt := `
CREATE TABLE IF NOT EXISTS images_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    image_id INTEGER NOT NULL,
    description VARCHAR(255),
    FOREIGN KEY (image_id) REFERENCES images (id) ON DELETE CASCADE,
    UNIQUE (name)
);`
	_, err := tx.Exec(stmt)
	return err
}

func updateFromV0(tx *sql.Tx) error {
	// v0..v1 the dawn of containers
	stmt := `
CREATE TABLE certificates (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    fingerprint VARCHAR(255) NOT NULL,
    type INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    certificate TEXT NOT NULL,
    UNIQUE (fingerprint)
);
CREATE TABLE containers (
    id INTEGER primary key AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    architecture INTEGER NOT NULL,
    type INTEGER NOT NULL,
    UNIQUE (name)
);
CREATE TABLE containers_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    container_id INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (container_id) REFERENCES containers (id),
    UNIQUE (container_id, key)
);
CREATE TABLE images (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    fingerprint VARCHAR(255) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    size INTEGER NOT NULL,
    public INTEGER NOT NULL DEFAULT 0,
    architecture INTEGER NOT NULL,
    creation_date DATETIME,
    expiry_date DATETIME,
    upload_date DATETIME NOT NULL,
    UNIQUE (fingerprint)
);
CREATE TABLE images_properties (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    image_id INTEGER NOT NULL,
    type INTEGER NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    FOREIGN KEY (image_id) REFERENCES images (id)
);`
	_, err := tx.Exec(stmt)
	return err
}
