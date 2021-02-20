// Copyright 2019-present Open Networking Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/atomix/go-framework/pkg/atomix/logging"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"math/big"
	"os"
	"runtime"
	"sigs.k8s.io/controller-runtime"
	"time"
)

var log = logging.GetLogger("atomix", "controller", "init")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {
	namespace := k8s.GetNamespace()
	service := k8s.GetName()
	webhookName := k8s.GetWebhookName()
	webhookPath := k8s.GetWebhookPath()

	printVersion()

	var caPEM, serverCertPEM, serverPrivKeyPEM *bytes.Buffer
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2020),
		Subject: pkix.Name{
			Organization: []string{"Open Networking Foundation"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		log.Panic(err)
	}

	caBytes, err := x509.CreateCertificate(cryptorand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Panic(err)
	}

	// PEM encode CA cert
	caPEM = new(bytes.Buffer)
	_ = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	dnsNames := []string{
		service,
		fmt.Sprintf("%s.%s", service, namespace),
		fmt.Sprintf("%s.%s.svc", service, namespace),
	}
	commonName := fmt.Sprintf("%s.%s.svc", service, namespace)

	cert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Open Networking Foundation"},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	serverPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		log.Panic(err)
	}

	serverCertBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Panic(err)
	}

	serverCertPEM = new(bytes.Buffer)
	_ = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})

	serverPrivKeyPEM = new(bytes.Buffer)
	_ = pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})

	err = WriteFile("/etc/webhook/certs/tls.crt", serverCertPEM)
	if err != nil {
		log.Panic(err)
	}

	err = WriteFile("/etc/webhook/certs/tls.key", serverPrivKeyPEM)
	if err != nil {
		log.Panic(err)
	}

	config := controllerruntime.GetConfigOrDie()
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panic(err)
	}

	sideEffects := admissionregistrationv1.SideEffectClassNone
	failurePolicy := admissionregistrationv1.Ignore
	scopeNamespaced := admissionregistrationv1.NamespacedScope
	var timeoutSeconds int32 = 30
	webhook := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookName,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: webhookName,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
							Scope:       &scopeNamespaced,
						},
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					},
				},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      service,
						Namespace: namespace,
						Path:      &webhookPath,
					},
					CABundle: caPEM.Bytes(),
				},
				AdmissionReviewVersions: []string{"v1beta1"},
				SideEffects:             &sideEffects,
				FailurePolicy:           &failurePolicy,
				TimeoutSeconds:          &timeoutSeconds,
			},
		},
	}

	if _, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(webhook); err != nil {
		if !errors.IsAlreadyExists(err) {
			log.Panic(err)
		}
	}
}

// WriteFile writes data in the file at the given path
func WriteFile(filepath string, sCert *bytes.Buffer) error {
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(sCert.Bytes())
	if err != nil {
		return err
	}
	return nil
}
