import yaml
import re
import os
import polling2
import json

from behave import given, then, when, step
from cluster import Cluster


provider_template = '''
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {name}
spec:
    package: xpkg.upbound.io/upbound/{name}:{version}
'''

dummy_server = '''
apiVersion: apps/v1
kind: Deployment
metadata:
  name: server-dummy
  namespace: crossplane-system
  labels:
    app: server-dummy
spec:
  replicas: 1
  selector:
      matchLabels:
        app: server-dummy
  template:
    metadata:
      labels:
        app: server-dummy
    spec:
      containers:
        - name: server
          image: ghcr.io/upbound/provider-dummy-server:main
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 9090
'''
dummy_service = '''
apiVersion: v1
kind: Service
metadata:
  namespace: crossplane-system
  name: server-dummy
spec:
  selector:
      app: server-dummy
  ports:
    - port: 80
      targetPort: 9090
      protocol: TCP

'''

provider_configs = {
    "provider-dummy": '''
apiVersion: dummy.upbound.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  endpoint: http://server-dummy.crossplane-system.svc.cluster.local    
    '''
}

@given(u'Crossplane is running in cluster')
def crossplane_installed(context):
    pass




def apply_resource(cluster, yaml_raw, namespace=None):
    res_yaml = yaml.full_load(yaml_raw)
    metadata = res_yaml["metadata"]
    metadata_name = metadata["name"]
    output = cluster.apply(yaml_raw, namespace)
    result = re.search(rf'.*{metadata_name}.*(created|unchanged|configured)', output)
    assert result is not None, f"Unable to apply YAML for CR '{metadata_name}': {output}"
    if namespace is not None:
        res_yaml["metadata"]["namespace"] = namespace
    return res_yaml


@step(u'Configuration is applied')
def apply_configuration(context):
    context.crossplane_configuration = apply_resource(context.cluster, context.text)


@then(u'configuration is marked as installed and healthy')
def configuration_install_healthy(context):
    name = context.crossplane_configuration["metadata"]["name"]
    context.cluster.wait_for_jq_match("configurations.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Healthy").status', "True")
    context.cluster.wait_for_jq_match("configurations.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Installed").status', "True")


@then(u'provider {name} is marked as installed and healthy')
def provider_installed_healthy(context, name):
    context.cluster.wait_for_jq_match("providers.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Healthy").status', "True")
    context.cluster.wait_for_jq_match("providers.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Installed").status', "True")


@then(u'provider {name} does not get installed')
def provider_not_installed(context, name):
    try:
        polling2.poll(lambda: context.cluster.get_resource_by_jq("providers.pkg.crossplane.io", name).strip(),
                      step=5, timeout=20, ignore_exceptions=(AssertionError,))
        assert False
    except polling2.TimeoutException as te:
        pass


@given(u'CompositeResourceDefinition is present')
@given(u'Composition is present')
def apply_yaml(context, namespace=None):
    return apply_resource(context.cluster, context.text)


@when(u'claim gets deployed')
def claim_deployed(context):
    context.claim = apply_resource(context.cluster, context.text, scenario_id(context))


@then(u'claim becomes synchronized and ready')
def claim_sync_ready(context):
    claim_type=f'{context.claim["kind"].lower()}s.{context.claim["apiVersion"].split("/")[0]}'
    context.cluster.wait_for_jq_match(claim_type, context.claim["metadata"]["name"], '.status.conditions[] | select(.type=="Synced").status', 'True', context.claim["metadata"]["namespace"])
    context.cluster.wait_for_jq_match(claim_type, context.claim["metadata"]["name"], '.status.conditions[] | select(.type=="Ready").status', 'True', context.claim["metadata"]["namespace"])



@then(u'claim composite resource becomes synchronized and ready')
def composite_resource_sync_ready(context):
    claim_type=f'{context.claim["kind"].lower()}s.{context.claim["apiVersion"].split("/")[0]}'
    composite_ref = polling2.poll(lambda: json.loads(context.cluster.get_resource_by_jq(claim_type, context.claim["metadata"]["name"], context.claim["metadata"]["namespace"], '.spec.resourceRef')),
                                  step=5, timeout=800, ignore_exceptions=(json.JSONDecodeError,))

    composite_ref_type=f'{composite_ref["kind"].lower()}s.{composite_ref["apiVersion"].split("/")[0]}'
    context.cluster.wait_for_jq_match(composite_ref_type, composite_ref["name"], '.status.conditions[] | select(.type=="Synced").status', 'True', context.claim["metadata"]["namespace"])
    context.cluster.wait_for_jq_match(composite_ref_type, composite_ref["name"], '.status.conditions[] | select(.type=="Ready").status', 'True', context.claim["metadata"]["namespace"])

    context.composite_ref = composite_ref
    context.composite_ref_type = composite_ref_type


@then(u'composed managed resources become ready and synchronized')
def managed_resource_sync_ready(context):
    cluster = Cluster()
    resource_refs = polling2.poll(lambda: json.loads(cluster.get_resource_by_jq(context.composite_ref_type, context.composite_ref["name"], context.claim["metadata"]["namespace"], '.spec.resourceRefs')),
                              step=5, timeout=800, ignore_exceptions=(json.JSONDecodeError,))
    for r in resource_refs:
        res_type = f'{r["kind"].lower()}.{r["apiVersion"].split("/")[0]}'
        context.cluster.wait_for_jq_match(res_type, r["name"], '.status.conditions[] | select(.type=="Synced").status', 'True')
        context.cluster.wait_for_jq_match(res_type, r["name"], '.status.conditions[] | select(.type=="Ready").status', 'True')



def scenario_id(context):
    return f"{os.path.basename(os.path.splitext(context.scenario.filename)[0]).lower()}-{context.scenario.line}"


