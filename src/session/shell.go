package session

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type YumParams struct {
	Update        bool     `json:"update"`
	Packages      []string `json:"packages"`
	GroupPackages []string `json:"group_packages"`
}

func (s *Session) Wget(url, option string) (bool, error) {
	if url == "" {
		url = "http://sample.local"
	}
	action := fmt.Sprintf("wget %s %s", option, url)
	rows := []ExpectRow{
		{Pattern: ` saved \[`, Reaction: ReactionSuccess, Arg: true, Label: "wget saved"},
		{Pattern: `ERROR 404: Not Found.`, Reaction: ReactionSuccess, Arg: false, Label: "wget 404"},
		{Pattern: `wget: unable to resolve host address`, Reaction: ReactionSuccess, Arg: false, Label: "wget resolve error"},
		{Pattern: `failed: No route to host.`, Reaction: ReactionSuccess, Arg: false, Label: "wget no route error"},
	}
	res, err := s.Do(action, rows, s.timeout)
	if err != nil {
		return false, err
	}
	_, _, err = s.child.Expect(regexp.MustCompile(s.CurrentNode().GetWaitPrompt()), s.timeout)
	if err != nil {
		return false, err
	}
	if val, ok := res.(bool); ok {
		return val, nil
	}
	return false, fmt.Errorf("unexpected wget result type")
}

func (s *Session) Patch(patchfile string) (bool, error) {
	action := fmt.Sprintf("patch < %s", patchfile)
	rows := []ExpectRow{
		{Pattern: `Reversed \(or previously applied\) patch detected!  Assume -R\? \[n\]`, Reaction: ReactionSendLine, Arg: "n", Label: "Reversed patch"},
		{Pattern: `Apply anyway\? \[n\]`, Reaction: ReactionSendLine, Arg: "n", Label: "Apply anyway prompt"},
		{Pattern: "", Reaction: ReactionSuccess, Arg: true, Label: "Patched"},
	}
	res, err := s.Do(action, rows, s.timeout)
	if err != nil {
		return false, err
	}
	if val, ok := res.(bool); ok {
		return val, nil
	}
	return false, nil
}

func (s *Session) YumInstall(params YumParams) error {
	rows := []ExpectRow{
		{Pattern: `Is this ok \[y/N\]:`, Reaction: ReactionSendLine, Arg: "y", Label: "yum prompt"},
		{Pattern: `Complete!`, Reaction: ReactionSuccess, Arg: true, Label: "yum complete"},
		{Pattern: `Nothing to do`, Reaction: ReactionSuccess, Arg: false, Label: "yum nothing"},
		{Pattern: `No packages in any requested group available to install or update`, Reaction: ReactionSuccess, Arg: false, Label: "yum no packages"},
		{Pattern: `No [Pp]ackages marked for [Uu]pdate`, Reaction: ReactionSuccess, Arg: false, Label: "yum no update"},
	}

	if params.Update {
		_, err := s.Do("sudo yum -y update", rows, s.timeout)
		if err != nil {
			return err
		}
		_, _, err = s.child.Expect(regexp.MustCompile(s.CurrentNode().GetWaitPrompt()), s.timeout)
		if err != nil {
			return err
		}
	}

	if len(params.Packages) > 0 {
		action := "sudo yum -y install " + strings.Join(params.Packages, " ")
		_, err := s.Do(action, rows, s.timeout)
		if err != nil {
			return err
		}
		_, _, err = s.child.Expect(regexp.MustCompile(s.CurrentNode().GetWaitPrompt()), s.timeout)
		if err != nil {
			return err
		}
	}

	if len(params.GroupPackages) > 0 {
		pkgStr := `"` + strings.Join(params.GroupPackages, `", "`) + `"`
		action := "sudo yum -y groupinstall " + pkgStr
		_, err := s.Do(action, rows, s.timeout)
		if err != nil {
			return err
		}
		_, _, err = s.child.Expect(regexp.MustCompile(s.CurrentNode().GetWaitPrompt()), s.timeout)
		if err != nil {
			return err
		}
	}
	return nil
}

func sedSeparator(srcExp, dstExp string) string {
	seps := []string{"/", "*", "%", ":", "@", "#"}
	for _, sep := range seps {
		if !strings.Contains(srcExp, sep) && !strings.Contains(dstExp, sep) {
			return sep
		}
	}
	return ""
}

func createSedEReplaceStr(srcExp, dstExp string) string {
	sep := sedSeparator(srcExp, dstExp)
	if sep == "" {
		return ""
	}
	if strings.Contains(srcExp, "'") {
		if strings.Contains(srcExp, `"`) {
			fmt.Fprintln(os.Stderr, "sed replace cannot handle single and double quote simultaneously in srcExp")
			os.Exit(1)
		}
		return fmt.Sprintf(`"s%s%s%s%s%s"`, sep, srcExp, sep, dstExp, sep)
	}
	return fmt.Sprintf(`'s%s%s%s%s%s'`, sep, srcExp, sep, dstExp, sep)
}

func sedReplaceStr(srcExp, dstStr, srcFile, dstFile string) string {
	sedEReplaceStr := createSedEReplaceStr(srcExp, dstStr)
	if dstFile == "" || srcFile == dstFile {
		return fmt.Sprintf("sed --in-place -e %s %s", sedEReplaceStr, srcFile)
	}
	return fmt.Sprintf("sed -e %s %s > %s", sedEReplaceStr, srcFile, dstFile)
}

func (s *Session) SedReplace(srcExp, dstStr, srcFile, dstFile string) (string, error) {
	return s.Run(sedReplaceStr(srcExp, dstStr, srcFile, dstFile))
}

func (s *Session) SedReplaceIfExists(srcExp, dstStr, srcFile, dstFile string) (string, error) {
	ifExistsStr := fmt.Sprintf("grep -q %s %s && ", srcExp, srcFile)
	return s.Run(ifExistsStr + sedReplaceStr(srcExp, dstStr, srcFile, dstFile))
}

func (s *Session) SedPipe(srcFile, dstFile string, expList [][2]string) (string, error) {
	var action string
	for i, exp := range expList {
		sedEReplaceStr := createSedEReplaceStr(exp[0], exp[1])
		if i == 0 {
			action = fmt.Sprintf("sed -e %s %s", sedEReplaceStr, srcFile)
		} else {
			action += fmt.Sprintf(" | sed -e %s", sedEReplaceStr)
		}
	}
	action += " > " + dstFile
	return s.Run(action)
}

func sedDeleteBetweenPatternStr(filePath, startExp, endExp string) string {
	sep := sedSeparator(startExp, endExp)
	return fmt.Sprintf("sed -ie '%s%s%s,%s%s%sd' %s", sep, startExp, sep, sep, endExp, sep, filePath)
}

func (s *Session) DeleteBetweenPattern(filePath, startExp, endExp string) (string, error) {
	if startExp == "" {
		startExp = "^####PLUR_START"
	}
	if endExp == "" {
		endExp = "^####PLUR_END"
	}
	return s.Run(sedDeleteBetweenPatternStr(filePath, startExp, endExp))
}

func sedAppendAfterPatternStr(filePath, exp, line string) string {
	sep := sedSeparator(exp, exp)
	command := "G"
	if line != "" {
		command = "a " + line
	}
	return fmt.Sprintf("sed -ie '%s%s%s%s' %s", sep, exp, sep, command, filePath)
}

func (s *Session) AppendLineAfterMatch(filePath, exp, line string) (string, error) {
	return s.Run(sedAppendAfterPatternStr(filePath, exp, line))
}

func (s *Session) AppendLineAfterMatchIfNotExists(filePath, exp, line string) (string, error) {
	grepExist := fmt.Sprintf("grep -qe '%s' %s", exp, filePath)
	action := fmt.Sprintf("%s || %s", grepExist, sedAppendAfterPatternStr(filePath, exp, line))
	return s.Run(action)
}

func (s *Session) AppendLine(line, filePath string) error {
	reLine := strings.ReplaceAll(line, "$", "\\$")
	ok, err := s.CountByEgrep(reLine, filePath)
	if err != nil {
		return err
	}
	if !ok {
		_, err = s.Run(fmt.Sprintf("echo '%s' >> %s", line, filePath))
		return err
	}
	return nil
}

func (s *Session) AppendBashrc(line string) error {
	bashrc := "$HOME/.bashrc"
	reLine := strings.ReplaceAll(line, "$", "\\$")
	ok, err := s.CountByEgrep(reLine, bashrc)
	if err != nil {
		return err
	}
	if !ok {
		_, err = s.Run(fmt.Sprintf("echo '%s' >> %s", line, bashrc))
		if err != nil {
			return err
		}
	}
	_, err = s.Run("source " + bashrc)
	return err
}

func (s *Session) AppendLines(filePath string, lines []string) error {
	_, err := s.CreateBackup(filePath, ".org", false)
	if err != nil {
		return err
	}
	for _, line := range lines {
		_, err = s.Run(fmt.Sprintf("echo %s >> %s", line, filePath))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) IdempotentAppend(filePath, expression, line string) (string, error) {
	action := fmt.Sprintf("grep -q '%s' %s || echo '%s' >> %s", expression, filePath, line, filePath)
	return s.Run(action)
}

func (s *Session) CountByEgrep(expression, filename string) (bool, error) {
	test := fmt.Sprintf("if [ `egrep -c '%s' %s` -gt 0 ]; ", expression, filename)
	return s.CheckTest(test)
}

func (s *Session) CheckTest(test2 string) (bool, error) {
	capt, err := s.Run(fmt.Sprintf(`%s && echo "Ye""sExists"`, test2))
	if err != nil {
		return false, err
	}
	return strings.Contains(capt, "YesExists"), nil
}

func (s *Session) CheckLineExistsInFile(filePath, exp string) (bool, error) {
	return s.CheckTest(fmt.Sprintf("grep -q '%s' %s", exp, filePath))
}

func (s *Session) CheckExists(name string) (bool, error) {
	return s.CheckTest(fmt.Sprintf("[ -e %s ]", name))
}

func (s *Session) CheckCommandExists(command string) (bool, error) {
	return s.CheckTest(fmt.Sprintf(`[ ! -z "$(command -v %s)" ]`, command))
}

func (s *Session) CheckFileExists(filename string) (bool, error) {
	return s.CheckTest(fmt.Sprintf("[ -f %s ]", filename))
}

func (s *Session) CheckDirExists(dirPath string, isFilePath bool) (bool, error) {
	if isFilePath {
		dirPath = fmt.Sprintf("`dirname %s`", dirPath)
	}
	return s.CheckTest(fmt.Sprintf("[ -d %s ]", dirPath))
}

func (s *Session) CheckYesOrNo(evalStr string) (bool, error) {
	return s.CheckTest(fmt.Sprintf("[ %s ]", evalStr))
}

func (s *Session) ServiceOn(service string) (string, error) {
	return s.Run(fmt.Sprintf("sudo systemctl enable --now %s", service))
}

func (s *Session) ServiceOff(service string) (string, error) {
	return s.Run(fmt.Sprintf("sudo systemctl disable --now %s", service))
}

func (s *Session) WorkOn(workDir string, isFilePath bool) (string, error) {
	if isFilePath {
		workDir = fmt.Sprintf("`dirname %s`", workDir)
	}
	return s.Run(fmt.Sprintf("[ -d %s ] || mkdir -p %s; cd %s", workDir, workDir, workDir))
}

func (s *Session) CreateDir(dirPath string, isFilePath bool, sudo bool) (string, error) {
	if isFilePath {
		dirPath = fmt.Sprintf("`dirname %s`", dirPath)
	}
	sudoStr := ""
	if sudo {
		sudoStr = "sudo"
	}
	return s.Run(fmt.Sprintf("[ -d %s ] || %s mkdir -p %s", dirPath, sudoStr, dirPath))
}

func (s *Session) CreateBackup(filename string, backupExt string, sudo bool) (string, error) {
	if backupExt == "" {
		backupExt = ".org"
	}
	sudoStr := ""
	if sudo {
		sudoStr = "sudo"
	}
	return s.Run(fmt.Sprintf("[ -f %s%s ] || %s cp %s %s%s", filename, backupExt, sudoStr, filename, filename, backupExt))
}

func (s *Session) HereDoc(dstFilePath, contents, eof string) error {
	if eof == "" {
		eof = "'EOF'"
	}
	curret := []ExpectRow{
		{Pattern: `> `, Reaction: ReactionSuccess, Arg: nil, Label: "HereDoc line prompt"},
	}
	_, err := s.Do(fmt.Sprintf("cat << %s > %s", eof, dstFilePath), curret, s.timeout)
	if err != nil {
		return err
	}

	lines := strings.Split(contents, "\n")
	for _, line := range lines {
		_, err = s.Do(line, curret, s.timeout)
		if err != nil {
			return err
		}
	}

	cleanEOF := strings.NewReplacer(``+`"`, "", `'`, "").Replace(eof)
	_, err = s.Run(cleanEOF)
	return err
}

func (s *Session) IsPingOk(hostnameOrIP string, count int) (bool, error) {
	if count <= 0 {
		count = 2
	}
	action := fmt.Sprintf("ping -c %d %s", count, hostnameOrIP)
	capture, err := s.Run(action)
	if err != nil {
		return false, err
	}
	baseMsg := `\d{1,2} packets transmitted, \d{1,2} received,`
	trueMsg := baseMsg + ` 0% packet loss, time `
	matched, _ := regexp.MatchString(trueMsg, capture)
	return matched, nil
}
