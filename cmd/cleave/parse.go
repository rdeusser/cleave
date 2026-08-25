package main

type ParseCmd struct {
	File string `arg:"" type:"existingfile" help:"Path to .clv file."`
}

func (cmd *ParseCmd) Run() error {
	_, _, err := compileFile(cmd.File)
	return err
}
