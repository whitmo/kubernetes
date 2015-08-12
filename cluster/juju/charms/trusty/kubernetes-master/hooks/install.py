#!/usr/bin/env python

# Copyright 2015 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import setup
setup.pre_install()
import subprocess

from charmhelpers import fetch
from charmhelpers.core import hookenv
from charmhelpers.contrib import ssl
from charmhelpers.fetch import archiveurl
from path import path


def install():
    install_packages()
    create_cert()
    hookenv.log('Installing go')
    download_go()

    hookenv.log('Adding kubernetes and go to the path')
    strings = [
        'export GOROOT=/usr/local/go\n',
        'export PATH=$PATH:$GOROOT/bin\n',
        'export KUBE_MASTER_IP=0.0.0.0\n',
        'export KUBERNETES_MASTER=http://$KUBE_MASTER_IP\n',
        ]
    update_rc_files(strings)
    hookenv.log('Downloading kubernetes code')
    clone_repository()

    hookenv.open_port(8080)
    hookenv.open_port(443)

    hookenv.log('Install complete')


def download_go():
    """
    Kubernetes charm strives to support upstream. Part of this is installing a
    fairly recent edition of GO. This fetches the golang archive and installs
    it in /usr/local
    """
    go_url = 'https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz'
    go_sha1 = '5020af94b52b65cc9b6f11d50a67e4bae07b0aff'
    handler = archiveurl.ArchiveUrlFetchHandler()
    handler.install(go_url, '/usr/local', go_sha1, 'sha1')


def clone_repository():
    """
    Clone the upstream repository into /opt/kubernetes for deployment compilation
    of kubernetes. Subsequently used during upgrades.
    """

    repository = 'https://github.com/GoogleCloudPlatform/kubernetes.git'
    kubernetes_directory = path('/opt/kubernetes')
    # Since we can not clone twice, check for the directory and remove it.
    if kubernetes_directory.isdir():
        kubernetes_directory.rmtree_p()

    command = ['git', 'clone', repository, kubernetes_directory]
    print(command)
    output = subprocess.check_output(command)
    print(output)


def install_packages():
    """
    Install required packages to build the k8s source, and syndicate between
    minion nodes. In addition, fetch pip to handle python dependencies
    """
    hookenv.log('Installing Debian packages')
    # Create the list of packages to install.
    apt_packages = ['build-essential',
                    'git',
                    'make',
                    'nginx',
                    'python-pip',]
    fetch.apt_install(fetch.filter_installed_packages(apt_packages))


def create_cert():
    """
    Create the certificate for the k8s tls enablement.
    """
    common_name = hookenv.unit_get('public-address')

    srv_kube = path('/srv/kubernetes')
    if not srv_kube.isdir():
        srv_kube.makedirs_p()
    # The cert and key go in the nginx directory so the template can resolve.
    cert_file = srv_kube / 'server.crt'
    key_file = srv_kube / 'server.key'
    if cert_file.exists():
        hookenv.log('Keeping the exising certificate.', 'WARNING')
        return
    else:
        hookenv.log('Generating self signed certificate.', 'INFO')
        # Generate the self signed certificate.
        ssl.generate_selfsigned(key_file, cert_file, cn=common_name)


def update_rc_files(strings):
    """
    Preseed the bash environment for ubuntu and root with K8's env vars to
    make interfacing with the api easier. (see: kubectrl docs)
    """
    rc_files = [path('/home/ubuntu/.bashrc'), path('/root/.bashrc')]
    for rc_file in rc_files:
        lines = rc_file.lines()
        for string in strings:
            if string not in lines:
                lines.append(string)
        rc_file.write_lines(lines)



if __name__ == "__main__":
    install()
