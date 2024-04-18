-- This file is Free Software under the Apache-2.0 License
-- without warranty, see README.md and LICENSES/Apache-2.0.txt for details.
--
-- SPDX-License-Identifier: Apache-2.0
--
-- SPDX-FileCopyrightText: 2024 German Federal Office for Information Security (BSI) <https://www.bsi.bund.de>
-- Software-Engineering: 2024 Intevation GmbH <https://intevation.de>

CREATE TABLE filters (
    id          int     PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
    definer     varchar NOT NULL,
    global      boolean NOT NULL DEFAULT FALSE,
    name        varchar NOT NULL,
    description varchar NOT NULL,
    query       varchar NOT NULL,
    num         int     NOT NULL GENERATED BY DEFAULT AS IDENTITY,
    CHECK(name <> ''),
    UNIQUE (definer, name)
);

CREATE TABLE filters_columns (
    filters_id int     NOT NULL REFERENCES filters (id) ON DELETE CASCADE,
    num        int     NOT NULL GENERATED BY DEFAULT AS IDENTITY,
    name       varchar NOT NULL,
    CHECK(name <> ''),
    UNIQUE (filters_id, num)
);

CREATE TABLE filters_orders (
    filters_id int     NOT NULL REFERENCES filters (id) ON DELETE CASCADE,
    num        int     NOT NULL GENERATED BY DEFAULT AS IDENTITY,
    name       varchar NOT NULL,
    ascending  bool    NOT NULL DEFAULT TRUE,
    CHECK(name <> ''),
    UNIQUE (filters_id, num)
);

--
-- permissions
--
GRANT INSERT, DELETE, SELECT, UPDATE ON filters         TO {{ .User | sanitize }};
GRANT INSERT, DELETE, SELECT, UPDATE ON filters_columns TO {{ .User | sanitize }};
GRANT INSERT, DELETE, SELECT, UPDATE ON filters_orders  TO {{ .User | sanitize }};
