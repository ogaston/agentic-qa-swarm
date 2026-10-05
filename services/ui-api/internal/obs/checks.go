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

// ReadableEventsFile comprueba que el archivo de eventos se puede leer. Si todavía no existe
// (go-intake lo crea con el primer evento; el suscriptor lo trata como «sin eventos»), exige
// que su directorio exista y sea un directorio.
func ReadableEventsFile(path string) error {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		d, derr := os.Stat(filepath.Dir(path))
		if derr != nil {
			return derr
		}
		if !d.IsDir() {
			return errors.New("el directorio del archivo de eventos no es un directorio")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s no es un archivo regular", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return f.Close()
}

// ReadyChecks son los chequeos de /readyz de ui-api: directorio de datos escribible, archivo de
// eventos legible y verificador de tokens configurado. verifierSet es un invariante de arranque
// (httpapi.New rechaza un verificador nulo): el chequeo es defensa en profundidad y se prueba
// inyectando false.
func ReadyChecks(dataDir, eventsFile string, verifierSet func() bool) []Check {
	return []Check{
		{Name: "data_dir", Fn: func(context.Context) error { return DirWritable(dataDir) }},
		{Name: "events_file", Fn: func(context.Context) error { return ReadableEventsFile(eventsFile) }},
		{Name: "token_verifier", Fn: func(context.Context) error {
			if verifierSet == nil || !verifierSet() {
				return errors.New("verificador de tokens no configurado")
			}
			return nil
		}},
	}
}
