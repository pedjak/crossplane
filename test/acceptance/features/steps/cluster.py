import base64

from steps.command import Command
import polling2
import json


class Cluster(object):
    def __init__(self):
        self.cmd = Command()
        self.cli = "kubectl"


    def apply(self, yaml, namespace=None, user=None):
        if namespace is not None:
            (output, exit_code) = self.cmd.run(f"{self.cli} create ns {namespace} --dry-run=client -o yaml | {self.cli} apply -f -")
            assert exit_code == 0, f"Non-zero exit code ({exit_code}) while applying a YAML: {output}"
            ns_arg = f"-n {namespace}"
        else:
            ns_arg = ""
        if user is not None:
            user_arg = f"--user={user}"
        else:
            user_arg = ""
        (output, exit_code) = self.cmd.run(f"{self.cli} apply {ns_arg} {user_arg} --validate=false -f -", yaml)
        assert exit_code == 0, f"Non-zero exit code ({exit_code}) while applying a YAML: {output}"
        return output


    def get_resource_by_jq(self, resource_type, name, namespace="default", jq_expression=None):
        if jq_expression is None:
            output, exit_code = self.cmd.run(f'{self.cli} get {resource_type} {name} -n {namespace} -o json')
        else:
            output, exit_code = self.cmd.run(f'{self.cli} get {resource_type} {name} -n {namespace} -o json | jq  \'{jq_expression}\'')
        assert exit_code == 0
        return output


    def wait_for_jq_match(self, resource_type, name, jq_expression, value, namespace="default"):
        polling2.poll(lambda: json.loads(
            self.get_resource_by_jq(resource_type, name, namespace, jq_expression)) == value,
                      step=5, timeout=800, ignore_exceptions=(json.JSONDecodeError,AssertionError,))
