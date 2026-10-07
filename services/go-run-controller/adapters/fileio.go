package adapters

import "os"

// appendFile es lo que los adaptadores de disco necesitan de un archivo abierto en modo append.
// *os.File lo cumple; las pruebas inyectan uno que falla a media escritura o en Sync.
type appendFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Truncate(size int64) error
	Close() error
}

type openAppendFunc func(path string) (appendFile, error)

func osOpenAppend(path string) (appendFile, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}

// appendDurable añade data (el archivo mide size) y lo sincroniza. Ante cualquier fallo de Write o
// Sync devuelve el archivo a size bytes (y lo sincroniza): no quedan bytes huérfanos ni una línea
// que el llamador cree ausente. intact=false si no pudo garantizarlo (el llamador debe darse por roto).
func appendDurable(f appendFile, size int64, data []byte) (err error, intact bool) {
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		return nil, true
	}
	if f.Truncate(size) != nil || f.Sync() != nil {
		return err, false
	}
	return err, true
}
