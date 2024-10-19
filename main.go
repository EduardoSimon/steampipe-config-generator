package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unicrons/steampipe-config-generator/cmd"
	"github.com/unicrons/steampipe-config-generator/pkg/aws"
	"github.com/unicrons/steampipe-config-generator/pkg/logger"

	log "github.com/sirupsen/logrus"
)

//go:embed templates/*.tmpl
var templates embed.FS

type CredentialAccount struct {
	Name             string
	RoleARN          string
	CredentialSource string
	ImportSchema     string
	DefaultRegion    string
	TargetRegions    []string
}

type ConnectionsTemplateData struct {
	Accounts []CredentialAccount
	Tags     map[string][]string
}

const defaultTmplFile = "templates/aws_connections.tmpl"

func createAWSCredentialsFile(credentialPath string, organizationAccounts []CredentialAccount) error {
	tmplFile := "templates/aws_credentials.tmpl"

	t, err := template.ParseFS(templates, tmplFile)
	if err != nil {
		return fmt.Errorf("error parsing template: %v", err)
	}

	err = os.MkdirAll(credentialPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating aws credentials path: %v", err)
	}
	filePath := filepath.Join(credentialPath, "credentials")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating aws credentials file: %v", err)
	}
	defer file.Close()

	err = t.Execute(file, organizationAccounts)
	if err != nil {
		return fmt.Errorf("error executing template: %v", err)
	}

	log.Debug("AWS credentials file created in:", filePath)
	return nil
}

func createAWSConnectionsFile(connectionsPath, templatePath string, data ConnectionsTemplateData) error {
	t, err := parseTemplate(templatePath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(connectionsPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating aws credentials path: %v", err)
	}
	filePath := filepath.Join(connectionsPath, "aws.spc")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating aws connections file: %v", err)
	}
	defer file.Close()

	err = t.Execute(file, data)
	if err != nil {
		return fmt.Errorf("error executing template: %v", err)
	}

	log.Debug("AWS Connections file created in:", filePath)
	return nil
}

func parseTemplate(templatePath string) (*template.Template, error) {
	if templatePath == "" {
		return template.ParseFS(templates, defaultTmplFile)
	} else {
		return template.ParseFiles(templatePath)
	}
}

func main() {
	flags, err := cmd.ParseFlags()

	if err != nil {
		log.Error("error parsing flags:", err)
		return
	}

	log.Debug("parsed flags:", flags)

	roleName := flags.RoleName
	credentialSource := flags.CredentialSource
	credentialPath := flags.CredentialPath
	connectionsPath := flags.ConnectionsPath
	importSchema := flags.ImportSchema
	defaultRegion := flags.DefaultRegion
	targetRegions := flags.TargetRegions
	assumeRoleArn := flags.AssumeRoleArn
	templatePath := flags.TemplatePath
	skipOUs := flags.SkipOUs
	logFormat := flags.LogFormat

	logger.SetLoggerFormat(logFormat)

	accounts, err := aws.GetOrganizationAccounts(assumeRoleArn, defaultRegion)
	if err != nil {
		log.Error("error getting aws organization accounts:", err)
		return
	}

	var organizationAccounts []CredentialAccount
	taggedAccounts := make(map[string][]string)

	for _, acc := range accounts {
		if slices.Contains(skipOUs, acc.AccountOU) {
			log.Infof("Skipping account %v included skipOUs argument", acc.AccountID)
			continue
		}

		name := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(acc.Name, " ", "_"), "-", "_"))

		for key, value := range acc.Tags {
			tagKey := key + "," + value
			taggedAccounts[tagKey] = append(taggedAccounts[tagKey], name)
		}

		organizationAccounts = append(organizationAccounts, CredentialAccount{
			Name:             name,
			RoleARN:          "arn:aws:iam::" + acc.AccountID + ":role/" + roleName,
			CredentialSource: credentialSource,
			ImportSchema:     importSchema,
			DefaultRegion:    defaultRegion,
			TargetRegions:    targetRegions,
		})
	}

	data := ConnectionsTemplateData{
		Accounts: organizationAccounts,
		Tags:     taggedAccounts,
	}

	err = createAWSCredentialsFile(credentialPath, organizationAccounts)
	if err != nil {
		log.Error("error creating aws credentials file:", err)
		return
	}

	err = createAWSConnectionsFile(connectionsPath, templatePath, data)
	if err != nil {
		log.Error("error creating aws connections file:", err)
		return
	}

	log.Info("config files created successfully")
}
