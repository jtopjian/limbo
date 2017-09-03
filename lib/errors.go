package lib

type ErrObjectDoesNotExist struct {
	msg string
}

func (e ErrObjectDoesNotExist) Error() string {
	e.msg = "Object does not exist"
	return e.msg
}
