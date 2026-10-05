package obs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DirWritable comprueba que dir existe y se puede escribir: crea y borra un archivo temporal.
func DirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".readyz-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

// AppendableFile comprueba que el archivo de eventos se puede abrir para agregar sin crearlo ni
// escribir en él; si todavía no existe (se crea con el primer evento), exige que su directorio
// sea escribible.
func AppendableFile(path string) error {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return DirWritable(filepath.Dir(path))
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s no es un archivo regular", filepath.Base(path))
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// ReadyChecks son los chequeos de /readyz de go-intake: directorio de datos escribible,
// archivo de eventos (outbox) escribible y secreto del webhook configurado.
// El secreto es además un invariante de arranque (run() no arranca sin él): el chequeo es
// defensa en profundidad y se prueba inyectando un secreto vacío.
func ReadyChecks(dataDir, eventsFile string, secret []byte) []Check {
	return []Check{
		{Name: "data_dir", Fn: func(context.Context) error { return DirWritable(dataDir) }},
		{Name: "outbox", Fn: func(context.Context) error { return AppendableFile(eventsFile) }},
		{Name: "webhook_secret", Fn: func(context.Context) error {
			if len(secret) == 0 {
				return errors.New("secreto del webhook no configurado")
			}
			return nil
		}},
	}
}
