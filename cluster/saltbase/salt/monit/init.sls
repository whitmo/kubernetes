{% if grains['os_family'] != 'RedHat' %}

monit:
  pkg:
    - installed

/etc/monit/conf.d/docker:
  file:
    - managed
    - source: salt://monit/docker
    - user: root
    - group: root
    - mode: 644

/etc/monit/conf.d/kubelet:
  file:
    - managed
    - source: salt://monit/kubelet
    - user: root
    - group: root
    - mode: 644

/etc/monit/conf.d/kube-proxy:
  file:
    - managed
    - source: salt://monit/kube-proxy
    - user: root
    - group: root
    - mode: 644

/etc/monit/monit_watcher.sh:
  file.managed:
    - source: salt://monit/monit_watcher.sh
    - user: root
    - group: root
    - mode: 755

crontab -l | { cat; echo "* * * * * /etc/monit/monit_watcher.sh 2>&1 | logger"; } | crontab -:
  cmd.run:
  - unless: crontab -l | grep "* * * * * /etc/monit/monit_watcher.sh 2>&1 | logger"

monit-service:
  service:
    - running
    - name: monit
    - watch:
      - pkg: monit
      - file: /etc/monit/conf.d/*

{% endif %}
