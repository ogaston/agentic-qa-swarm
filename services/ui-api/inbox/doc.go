// Package inbox mantiene la proyeccion de la bandeja de notificaciones de
// ui-api (C1/C8): construye entradas pending a partir de eventos
// notify.created, las lista y registra confirmaciones en un JSONL solo-agregar.
// Publicar run.confirmed no es de este paquete (U1-T07).
package inbox
