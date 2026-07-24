package mcp

// MRTR (Multi-Round Turn Request - SEP-2322)
// Especificación para interrupción condicional y recopilación de parámetros del usuario.

type InputRequestType string

const (
	InputTypePrompt       InputRequestType = "prompt"
	InputTypeConfirmation InputRequestType = "confirmation"
	InputTypeCredentials  InputRequestType = "credentials"
)

type InputRequest struct {
	ID           string           `json:"id" jsonschema:"Identificador único de la petición de entrada"`
	Prompt       string           `json:"prompt" jsonschema:"Mensaje o pregunta dirigida al usuario"`
	Type         InputRequestType `json:"type,omitempty" jsonschema:"Tipo de entrada esperada (prompt, confirmation, credentials)"`
	Required     bool             `json:"required" jsonschema:"Indica si el parámetro es obligatorio"`
	DefaultValue string           `json:"default_value,omitempty" jsonschema:"Valor por defecto si aplica"`
}

type InputRequiredResult struct {
	Status        string         `json:"status" jsonschema:"Estado de interrupción (input_required)"`
	Message       string         `json:"message" jsonschema:"Resumen descriptivo de la solicitud"`
	InputRequests []InputRequest `json:"inputRequests" jsonschema:"Lista de parámetros requeridos del cliente/usuario"`
}

func NewInputRequiredResult(msg string, requests ...InputRequest) InputRequiredResult {
	return InputRequiredResult{
		Status:        "input_required",
		Message:       msg,
		InputRequests: requests,
	}
}
