-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Temperatures recorded under the name the kernel gives each sensor, not the
-- one a board profile gives it, so renaming a reading keeps its history. A
-- box with no profile already recorded under the kernel's names; the DK
-- recorded under its profile's, and its series move with every point they
-- hold. Only a DK's file holds temp.cluster, the name its profile gave the
-- CPU, so no other board's series move. Where a series already stands under
-- the kernel's name, its own points stay and the older ones join it.

CREATE TEMP TABLE rekey (old TEXT PRIMARY KEY, new TEXT NOT NULL);
INSERT INTO rekey (old, new)
	SELECT column1, column2 FROM (VALUES
		('temp.cluster', 'temp.cluster-thermal'),
		('temp.Memory', 'temp.ddr-thermal'),
		('temp.Network', 'temp.fman-thermal'),
		('temp.SerDes', 'temp.serdes-thermal'),
		('temp.Crypto', 'temp.sec-thermal'),
		('temp.PCB near CPU', 'temp.temp1.soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c'),
		('temp.CPU (external diode)', 'temp.temp2.soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c'),
		('temp.SFP+ modules', 'temp.temp1.soc/i2c@2180000/i2c-mux@70/i2c@2/temp-sensor@4c')
	)
	WHERE EXISTS (SELECT 1 FROM series WHERE name = 'temp.cluster');

INSERT INTO series (name)
	SELECT rekey.new FROM rekey JOIN series ON series.name = rekey.old
	WHERE NOT EXISTS (SELECT 1 FROM series AS taken WHERE taken.name = rekey.new);

INSERT OR IGNORE INTO points (series_id, resolution, time, average, peak)
	SELECT moved.id, points.resolution, points.time, points.average, points.peak
	FROM rekey
	JOIN series AS was ON was.name = rekey.old
	JOIN series AS moved ON moved.name = rekey.new
	JOIN points ON points.series_id = was.id;

DELETE FROM points WHERE series_id IN (SELECT was.id FROM rekey JOIN series AS was ON was.name = rekey.old);
DELETE FROM series WHERE name IN (SELECT old FROM rekey);
DROP TABLE rekey;
