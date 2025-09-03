CREATE TABLE IF NOT EXISTS students (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    hidden BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    main_student INTEGER NOT NULL,
    assistant_student INTEGER NOT NULL,
    date INTEGER NOT NULL, -- yyyymmdd
    FOREIGN KEY (main_student) REFERENCES students(id),
    FOREIGN KEY (assistant_student) REFERENCES students(id)
);