package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-sqlite3"
)

var (
	appStyle  = lipgloss.NewStyle().Padding(1, 2)
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

const dbFile = "students.db"

// --- Modelos de Datos ---

type Student struct {
	ID                 int
	Name               string
	LastAssignmentDate sql.NullInt64
	Hidden             bool
}

func (s Student) Title() string { return s.Name }
func (s Student) Description() string {
	return fmt.Sprintf("Última asignación: %s", formatDate(s.LastAssignmentDate)) }


func (s Student) FilterValue() string { return s.Name }

type Companion struct {
	ID                    int
	Name                  string
	LastTogetherDate      sql.NullInt64
	LastAnyAssignmentDate sql.NullInt64
}

func (c Companion) Title() string { return c.Name }
func (c Companion) Description() string {
	togetherDateStr := fmt.Sprintf("Juntos por última vez: %s", formatDate(c.LastTogetherDate))
	anyDateStr := fmt.Sprintf("Última asignación: %s", formatDate(c.LastAnyAssignmentDate))
	return fmt.Sprintf("%s | %s", togetherDateStr, anyDateStr)
}
func (c Companion) FilterValue() string { return c.Name }

// --- Bubble Tea Model ---

type appState int

const (
	stateStudentList appState = iota
	stateCompanionList
	stateAssigningDate
	stateAddingStudent
)

type model struct {
	db                *sql.DB
	list              list.Model
	textInput         textinput.Model
	state             appState
	selectedStudent   Student
	selectedCompanion Companion
	err               error
}

func initialModel(db *sql.DB) model {
	m := model{db: db, state: stateStudentList}

	// Configurar la lista
	delegate := list.NewDefaultDelegate()
	m.list = list.New(nil, delegate, 0, 0)
	m.list.Title = "Seleccionar Estudiante"
	m.list.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "seleccionar")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "añadir estudiante")),
			key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "ocultar/mostrar")),
		}
	}
	m.list.SetItems(fetchStudents(db))

	// Configurar el campo de texto (reutilizado para fechas y nombres)
	ti := textinput.New()
	ti.Focus()
	m.textInput = ti

	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		top, right, bottom, left := appStyle.GetPadding()
		m.list.SetSize(msg.Width-left-right, msg.Height-top-bottom)

	case tea.KeyMsg:
		// Si hay un error, cualquier tecla lo borra y vuelve al estado anterior
		if m.err != nil {
			m.err = nil
			return m, nil
		}

		switch m.state {
		case stateStudentList:
			if m.list.FilterState() == list.Filtering {
				break
			}
			switch keypress := msg.String(); keypress {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "a":
				m.state = stateAddingStudent
				m.textInput.Placeholder = "Nombre del nuevo estudiante"
				m.textInput.CharLimit = 50
				m.textInput.Width = 50
				m.textInput.SetValue("")
				return m, m.textInput.Focus()
			case "enter":
				// Solo procesar si hay items en la lista
				if len(m.list.Items()) > 0 {
					selected, ok := m.list.SelectedItem().(Student)
					if ok {
						m.selectedStudent = selected
						m.state = stateCompanionList
						m.list.Title = fmt.Sprintf("Asignar acompañante para: %s", m.selectedStudent.Name)
						m.list.SetItems(fetchCompanions(m.db, m.selectedStudent.ID))
					}
				}
			case "h":
				if len(m.list.Items()) > 0 {
					selected, ok := m.list.SelectedItem().(Student)
					if ok {
						err := toggleStudentHidden(m.db, selected.ID, !selected.Hidden)
						if err != nil {
							m.err = err
							return m, nil
						}
						m.list.SetItems(fetchStudents(m.db))
					}
				}
			}
		case stateCompanionList:
			switch keypress := msg.String(); keypress {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "enter":
				selected, ok := m.list.SelectedItem().(Companion)
				if ok {
					m.selectedCompanion = selected
					m.state = stateAssigningDate
					m.textInput.Placeholder = "YYYYMMDD"
					m.textInput.CharLimit = 8
					m.textInput.Width = 20
					m.textInput.SetValue(time.Now().Format("20060102"))
					return m, m.textInput.Focus()
				}
			}
		case stateAssigningDate:
			switch keypress := msg.String(); keypress {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "enter":
				dateStr := m.textInput.Value()
				date, err := strconv.Atoi(dateStr)
				if err != nil || len(dateStr) != 8 {
					m.err = fmt.Errorf("fecha inválida, debe ser formato YYYYMMDD")
					return m, nil
				}
				err = createAssignment(m.db, m.selectedStudent.ID, m.selectedCompanion.ID, date)
				if err != nil {
					m.err = err
					return m, nil
				}
				m.state = stateStudentList
				m.list.Title = "Seleccionar Estudiante"
				m.list.SetItems(fetchStudents(m.db))
			}
		case stateAddingStudent:
			switch keypress := msg.String(); keypress {
			case "ctrl+c", "q", "esc":
				m.state = stateStudentList
				m.err = nil
				return m, nil
			case "enter":
				studentName := m.textInput.Value()
				if studentName == "" {
					m.err = fmt.Errorf("el nombre no puede estar vacío")
					return m, nil
				}
				err := createStudent(m.db, studentName)
				if err != nil {
					m.err = err
					return m, nil
				}
				m.state = stateStudentList
				m.list.SetItems(fetchStudents(m.db))
			}
		}
	}

	switch m.state {
	case stateStudentList, stateCompanionList:
		m.list, cmd = m.list.Update(msg)
	case stateAssigningDate, stateAddingStudent:
		m.textInput, cmd = m.textInput.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	if m.err != nil {
		return appStyle.Render(fmt.Sprintf("Error: %v\n\nPresiona cualquier tecla para continuar...", m.err))
	}

	switch m.state {
	case stateAddingStudent:
		var b strings.Builder
		b.WriteString("Añadir Nuevo Estudiante\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Presiona Enter para guardar, Esc para cancelar."))
		return appStyle.Render(b.String())
	case stateAssigningDate:
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Asignando a %s con %s\n\n", m.selectedStudent.Name, m.selectedCompanion.Name))
		b.WriteString("Ingresa la fecha de la asignación (YYYYMMDD):\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Presiona Enter para confirmar, Ctrl+C para cancelar."))
		return appStyle.Render(b.String())
	default:
		return appStyle.Render(m.list.View())
	}
}

// --- Lógica de Base de Datos ---

func initDB(filepath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}

	schema, err := ioutil.ReadFile("schema.sql")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(string(schema))
	if err != nil {
		return nil, err
	}

	return db, nil
}

func createStudent(db *sql.DB, name string) error {
	_, err := db.Exec("INSERT INTO students (name) VALUES (?)", name)
	if err != nil {
		// Manejar error de unicidad de manera amigable
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("el estudiante '%s' ya existe", name)
		}
		return err
	}
	return nil
}

func toggleStudentHidden(db *sql.DB, studentID int, hidden bool) error {
	_, err := db.Exec("UPDATE students SET hidden = ? WHERE id = ?", hidden, studentID)
	return err
}

func fetchStudents(db *sql.DB) []list.Item {
	query := `
		SELECT s.id, s.name, s.hidden, MAX(a.date) as last_assignment_date
		FROM students s
		LEFT JOIN (
			SELECT main_student as student_id, date FROM assignments
			UNION ALL
			SELECT assistant_student as student_id, date FROM assignments
		) a ON s.id = a.student_id
		WHERE s.hidden = 0
		GROUP BY s.id, s.name
		ORDER BY last_assignment_date ASC, s.name;
	`
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error fetching students: %v", err)
		return []list.Item{}
	}
	defer rows.Close()

	var items []list.Item
	for rows.Next() {
		var s Student
		if err := rows.Scan(&s.ID, &s.Name, &s.Hidden, &s.LastAssignmentDate); err != nil {
			log.Printf("Error scanning student: %v", err)
			continue
		}
		items = append(items, s)
	}
	return items
}

func fetchCompanions(db *sql.DB, mainStudentID int) []list.Item {
	query := `
		SELECT
			s.id,
			s.name,
			(SELECT MAX(date) FROM assignments WHERE
				(main_student = ?1 AND assistant_student = s.id) OR
				(main_student = s.id AND assistant_student = ?1)
			) as last_together_date,
			(SELECT MAX(date) FROM assignments WHERE main_student = s.id OR assistant_student = s.id) as last_any_assignment_date
		FROM students s
		WHERE s.id != ?1 AND s.hidden = 0
		ORDER BY last_together_date ASC, last_any_assignment_date ASC;
	`
	rows, err := db.Query(query, mainStudentID)
	if err != nil {
		log.Printf("Error fetching companions: %v", err)
		return []list.Item{}
	}
	defer rows.Close()

	var items []list.Item
	for rows.Next() {
		var c Companion
		if err := rows.Scan(&c.ID, &c.Name, &c.LastTogetherDate, &c.LastAnyAssignmentDate); err != nil {
			log.Printf("Error scanning companion: %v", err)
			continue
		}
		items = append(items, c)
	}
	return items
}

func createAssignment(db *sql.DB, mainStudentID, assistantStudentID int, date int) error {
	_, err := db.Exec(
		"INSERT INTO assignments (main_student, assistant_student, date) VALUES (?, ?, ?)",
		mainStudentID,
		assistantStudentID,
		date,
	)
	return err
}

// --- Utilidades ---

func formatDate(d sql.NullInt64) string {
	if !d.Valid {
		return "Nunca"
	}
	dateStr := strconv.FormatInt(d.Int64, 10)
	if len(dateStr) != 8 {
		return "Fecha inválida"
	}
	return fmt.Sprintf("%s-%s-%s", dateStr[0:4], dateStr[4:6], dateStr[6:8])
}

// --- Main ---

func main() {
	db, err := initDB(dbFile)
	if err != nil {
		log.Fatalf("Error inicializando la base de datos: %v", err)
	}
	defer db.Close()

	p := tea.NewProgram(initialModel(db), tea.WithAltScreen())

	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}
