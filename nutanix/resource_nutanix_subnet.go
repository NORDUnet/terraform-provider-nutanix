package nutanix

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-nutanix/client/v3"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func resourceNutanixSubnet() *schema.Resource {
	return &schema.Resource{
		Create: resourceNutanixSubnetCreate,
		Read:   resourceNutanixSubnetRead,
		Update: resourceNutanixSubnetUpdate,
		Delete: resourceNutanixSubnetDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: getSubnetSchema(),
	}
}

func resourceNutanixSubnetCreate(d *schema.ResourceData, meta interface{}) error {
	//Get client connection
	conn := meta.(*NutanixClient).API

	// Prepare request
	request := &v3.SubnetIntentInput{}
	spec := &v3.Subnet{}
	metadata := &v3.SubnetMetadata{}
	subnet := &v3.SubnetResources{}

	//Read arguments and set request values
	n, nok := d.GetOk("name")
	desc, descok := d.GetOk("description")
	azr, azrok := d.GetOk("availability_zone_reference")
	cr, crok := d.GetOk("cluster_reference")
	_, stok := d.GetOk("subnet_type")

	if !stok && !nok {
		return fmt.Errorf("Please provide the required attributes name, subnet_type")
	}

	// Read Arguments and set request values
	if v, ok := d.GetOk("api_version"); ok {
		request.APIVersion = utils.String(v.(string))
	}
	if !nok {
		return fmt.Errorf("Please provide the required name attribute")
	}
	if err := getSubnetMetadaAttributes(d, metadata); err != nil {
		return err
	}
	if descok {
		spec.Description = utils.String(desc.(string))
	}
	if azrok {
		a := azr.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
		}
		if v, ok := a["name"]; ok {
			r.Name = utils.String(v.(string))
		}
		spec.AvailabilityZoneReference = r
	}
	if crok {
		a := cr.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
		}
		if v, ok := a["name"]; ok {
			r.Name = utils.String(v.(string))
		}
		spec.ClusterReference = r
	}

	if err := getSubnetResources(d, subnet); err != nil {
		return err
	}

	subnetUUID, err := resourceNutanixSubnetExists(conn, d.Get("name").(string))

	if err != nil {
		return err
	}

	if subnetUUID != nil {
		return fmt.Errorf("Subnet already with name %s exists in the given cluster, UUID %s", d.Get("name").(string), *subnetUUID)
	}

	spec.Name = utils.String(n.(string))
	spec.Resources = subnet
	request.Metadata = metadata
	request.Spec = spec

	utils.PrintToJSON(request, "CREATE METHOD REQUEST")

	//Make request to the API
	resp, err := conn.V3.CreateSubnet(request)
	if err != nil {
		return err
	}

	d.SetId(*resp.Metadata.UUID)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PENDING", "RUNNING"},
		Target:     []string{"COMPLETE"},
		Refresh:    subnetStateRefreshFunc(conn, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for subnet (%s) to create: %s", d.Id(), err)
	}

	return resourceNutanixSubnetRead(d, meta)
}

func resourceNutanixSubnetRead(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] Reading Subnet: %s", d.Get("name").(string))

	// Get client connection
	conn := meta.(*NutanixClient).API

	// Make request to the API
	resp, err := conn.V3.GetSubnet(d.Id())
	if err != nil {
		return err
	}

	// set metadata values
	metadata := make(map[string]interface{})
	metadata["last_update_time"] = resp.Metadata.LastUpdateTime.String()
	metadata["kind"] = utils.StringValue(resp.Metadata.Kind)
	metadata["uuid"] = utils.StringValue(resp.Metadata.UUID)
	metadata["creation_time"] = resp.Metadata.CreationTime.String()
	metadata["spec_version"] = strconv.Itoa(int(utils.Int64Value(resp.Metadata.SpecVersion)))
	metadata["spec_hash"] = utils.StringValue(resp.Metadata.SpecHash)
	metadata["name"] = utils.StringValue(resp.Metadata.Name)
	if err := d.Set("metadata", metadata); err != nil {
		return err
	}
	if err := d.Set("categories", resp.Metadata.Categories); err != nil {
		return err
	}

	or := make(map[string]interface{})
	if resp.Metadata.OwnerReference != nil {
		or["kind"] = utils.StringValue(resp.Metadata.OwnerReference.Kind)
		or["name"] = utils.StringValue(resp.Metadata.OwnerReference.Name)
		or["uuid"] = utils.StringValue(resp.Metadata.OwnerReference.UUID)

	}
	if err := d.Set("owner_reference", or); err != nil {
		return err
	}
	if err := d.Set("api_version", utils.StringValue(resp.APIVersion)); err != nil {
		return err
	}
	if err := d.Set("name", utils.StringValue(resp.Status.Name)); err != nil {
		return err
	}
	if err := d.Set("description", utils.StringValue(resp.Status.Description)); err != nil {
		return err
	}
	// set availability zone reference values
	availabilityZoneReference := make(map[string]interface{})
	if resp.Status.AvailabilityZoneReference != nil {
		availabilityZoneReference["kind"] = utils.StringValue(resp.Status.AvailabilityZoneReference.Kind)
		availabilityZoneReference["name"] = utils.StringValue(resp.Status.AvailabilityZoneReference.Name)
		availabilityZoneReference["uuid"] = utils.StringValue(resp.Status.AvailabilityZoneReference.UUID)
	}
	if err := d.Set("availability_zone_reference", availabilityZoneReference); err != nil {
		return err
	}
	// set cluster reference values
	clusterReference := make(map[string]interface{})
	if resp.Status.ClusterReference != nil {
		clusterReference["kind"] = utils.StringValue(resp.Status.ClusterReference.Kind)
		clusterReference["name"] = utils.StringValue(resp.Status.ClusterReference.Name)
		clusterReference["uuid"] = utils.StringValue(resp.Status.ClusterReference.UUID)
	}
	if err := d.Set("cluster_reference", clusterReference); err != nil {
		return err
	}
	// set state value
	if err := d.Set("state", utils.StringValue(resp.Status.State)); err != nil {
		return err
	}
	if err := d.Set("vswitch_name", utils.StringValue(resp.Status.Resources.VswitchName)); err != nil {
		return err
	}
	if resp.Status.Resources.SubnetType != nil {
		if err := d.Set("subnet_type", utils.StringValue(resp.Status.Resources.SubnetType)); err != nil {
			return err
		}
	} else {
		if err := d.Set("subnet_type", ""); err != nil {
			return err
		}
	}
	if resp.Status.Resources.IPConfig != nil {
		if err := d.Set("default_gateway_ip", utils.StringValue(resp.Status.Resources.IPConfig.DefaultGatewayIP)); err != nil {
			return err
		}
		if err := d.Set("prefix_length", utils.Int64Value(resp.Status.Resources.IPConfig.PrefixLength)); err != nil {
			return err
		}
		if err := d.Set("subnet_ip", utils.StringValue(resp.Status.Resources.IPConfig.SubnetIP)); err != nil {
			return err
		}
		address := make(map[string]interface{})
		port := int64(0)
		if resp.Status.Resources.IPConfig.DHCPServerAddress != nil {
			//set ip_config.dhcp_server_address
			address["ip"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPServerAddress.IP)
			address["fqdn"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPServerAddress.FQDN)
			address["ipv6"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPServerAddress.IPV6)
			port = utils.Int64Value(resp.Status.Resources.IPConfig.DHCPServerAddress.Port)
		}
		if err := d.Set("dhcp_server_address", address); err != nil {
			return err
		}
		if err := d.Set("dhcp_server_address_port", port); err != nil {
			return err
		}
		if resp.Status.Resources.IPConfig.PoolList != nil {
			pl := resp.Status.Resources.IPConfig.PoolList
			poolList := make([]string, len(pl))
			for k, v := range pl {
				poolList[k] = utils.StringValue(v.Range)
			}
			if err := d.Set("ip_config_pool_list_ranges", poolList); err != nil {
				return err
			}
		} else {
			if err := d.Set("ip_config_pool_list_ranges", make([]string, 0)); err != nil {
				return err
			}
		}
		if resp.Status.Resources.IPConfig.DHCPOptions != nil {
			//set dhcp_options
			dOptions := make(map[string]interface{})
			dOptions["boot_file_name"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPOptions.BootFileName)
			dOptions["domain_name"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPOptions.DomainName)
			dOptions["tftp_server_name"] = utils.StringValue(resp.Status.Resources.IPConfig.DHCPOptions.TFTPServerName)

			if err := d.Set("dhcp_options", dOptions); err != nil {
				return err
			}

			if resp.Status.Resources.IPConfig.DHCPOptions.DomainNameServerList != nil {
				dnsl := resp.Status.Resources.IPConfig.DHCPOptions.DomainNameServerList
				dnsList := make([]string, len(dnsl))
				for k, v := range dnsl {
					dnsList[k] = utils.StringValue(v)
				}
				if err := d.Set("dhcp_domain_name_server_list", dnsList); err != nil {
					return err
				}
			}
			if resp.Status.Resources.IPConfig.DHCPOptions.DomainSearchList != nil {
				dnsl := resp.Status.Resources.IPConfig.DHCPOptions.DomainSearchList
				dsList := make([]string, len(dnsl))
				for k, v := range dnsl {
					dsList[k] = utils.StringValue(v)
				}
				if err := d.Set("dhcp_domain_search_list", dsList); err != nil {
					return err
				}
			}
		} else {
			if err := d.Set("dhcp_options", make(map[string]interface{})); err != nil {
				return err
			}
			if err := d.Set("dhcp_domain_name_server_list", make([]map[string]interface{}, 0)); err != nil {
				return err
			}
			if err := d.Set("dhcp_domain_search_list", make([]map[string]interface{}, 0)); err != nil {
				return err
			}
		}
	} else {
		if err := d.Set("default_gateway_ip", ""); err != nil {
			return err
		}
		if err := d.Set("prefix_length", 0); err != nil {
			return err
		}
		if err := d.Set("subnet_ip", ""); err != nil {
			return err
		}
		if err := d.Set("dhcp_server_address_port", 0); err != nil {
			return err
		}
		if err := d.Set("ip_config_pool_list_ranges", make([]map[string]interface{}, 0)); err != nil {
			return err
		}
		if err := d.Set("dhcp_options", make(map[string]interface{})); err != nil {
			return err
		}
		if err := d.Set("dhcp_domain_name_server_list", make([]map[string]interface{}, 0)); err != nil {
			return err
		}
		if err := d.Set("dhcp_domain_search_list", make([]map[string]interface{}, 0)); err != nil {
			return err
		}
	}

	if err := d.Set("vlan_id", resp.Status.Resources.VlanID); err != nil {
		return err
	}
	// set network_function_chain_reference
	if resp.Status.Resources.NetworkFunctionChainReference != nil {
		nfcr := make(map[string]interface{})
		nfcr["kind"] = utils.StringValue(resp.Status.Resources.NetworkFunctionChainReference.Kind)
		nfcr["name"] = utils.StringValue(resp.Status.Resources.NetworkFunctionChainReference.Name)
		nfcr["uuid"] = utils.StringValue(resp.Status.Resources.NetworkFunctionChainReference.UUID)

		if err := d.Set("network_function_chain_reference", nfcr); err != nil {
			return err
		}
	} else {
		if err := d.Set("network_function_chain_reference", make(map[string]interface{})); err != nil {
			return err
		}
	}

	return nil
}

func resourceNutanixSubnetUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*NutanixClient).API

	log.Printf("Updating the subnet with the uuid %s", d.Id())
	fmt.Printf("Updating the subnet with the uuid %s", d.Id())

	request := &v3.SubnetIntentInput{}
	metadata := &v3.SubnetMetadata{}
	res := &v3.SubnetResources{}
	ipcfg := &v3.IPConfig{}
	dhcpO := &v3.DHCPOptions{}
	spec := &v3.Subnet{}

	// get state
	if d.HasChange("metadata") {
		m := d.Get("metadata")
		metad := m.(map[string]interface{})
		if v, ok := metad["uuid"]; ok && v != "" {
			metadata.UUID = utils.String(v.(string))
		}
		if v, ok := metad["spec_version"]; ok && v != 0 {
			i, err := strconv.Atoi(v.(string))
			if err != nil {
				return err
			}
			metadata.SpecVersion = utils.Int64(int64(i))
		}
		if v, ok := metad["spec_hash"]; ok && v != "" {
			metadata.SpecHash = utils.String(v.(string))
		}
		if v, ok := metad["name"]; ok {
			metadata.Name = utils.String(v.(string))
		}
	}

	if d.HasChange("categories") {
		p := d.Get("categories").(map[string]interface{})
		labels := map[string]string{}
		for k, v := range p {
			labels[k] = v.(string)
		}
		metadata.Categories = labels
	}
	if d.HasChange("owner_reference") {
		or := d.Get("owner_reference").(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(or["kind"].(string)),
			UUID: utils.String(or["uuid"].(string)),
			Name: utils.String(or["name"].(string)),
		}
		metadata.OwnerReference = r
	}
	if d.HasChange("project_reference") {
		pr := d.Get("project_reference").(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(pr["kind"].(string)),
			UUID: utils.String(pr["uuid"].(string)),
			Name: utils.String(pr["name"].(string)),
		}
		metadata.ProjectReference = r
	}
	if d.HasChange("name") {
		spec.Name = utils.String(d.Get("name").(string))
	}
	if d.HasChange("description") {
		spec.Description = utils.String(d.Get("description").(string))
	}
	if d.HasChange("availability_zone_reference") {
		a := d.Get("availability_zone_reference").(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
			Name: utils.String(a["name"].(string)),
		}
		spec.AvailabilityZoneReference = r
	}
	if d.HasChange("cluster_reference") {
		a := d.Get("cluster_reference").(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
			Name: utils.String(a["name"].(string)),
		}
		spec.ClusterReference = r
	}
	if d.HasChange("dhcp_domain_name_server_list") {
		dd := d.Get("dhcp_domain_name_server_list").([]string)
		ddn := make([]*string, len(dd))
		for k, v := range dd {
			ddn[k] = utils.String(v)
		}
		dhcpO.DomainNameServerList = ddn
	}
	if d.HasChange("dhcp_domain_search_list") {
		dd := d.Get("dhcp_domain_search_list").([]string)
		ddn := make([]*string, len(dd))
		for k, v := range dd {
			ddn[k] = utils.String(v)
		}
		dhcpO.DomainSearchList = ddn
	}
	if d.HasChange("ip_config_pool_list_ranges") {
		dd := d.Get("ip_config_pool_list_ranges").([]string)
		ddn := make([]*v3.IPPool, len(dd))
		for k, v := range dd {
			i := &v3.IPPool{}
			i.Range = utils.String(v)
			ddn[k] = i
		}
		ipcfg.PoolList = ddn
	}
	if d.HasChange("dhcp_options") {
		dOptions := d.Get("dhcp_options").(map[string]interface{})
		dhcpO.BootFileName = utils.String(dOptions["boot_file_name"].(string))
		dhcpO.DomainName = utils.String(dOptions["domain_name"].(string))
		dhcpO.TFTPServerName = utils.String(dOptions["tftp_server_name"].(string))
	}
	if d.HasChange("network_function_chain_reference") {
		a := d.Get("network_function_chain_reference").(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
			Name: utils.String(a["name"].(string)),
		}
		res.NetworkFunctionChainReference = r
	}
	if d.HasChange("vswitch_name") {
		res.VswitchName = utils.String(d.Get("vswitch_name").(string))
	}
	if d.HasChange("subnet_type") {
		res.SubnetType = utils.String(d.Get("subnet_type").(string))
	}
	if d.HasChange("default_gateway_ip") {
		ipcfg.DefaultGatewayIP = utils.String(d.Get("default_gateway_ip").(string))
	}
	if d.HasChange("prefix_length") {
		ipcfg.PrefixLength = utils.Int64(int64(d.Get("prefix_length").(int)))
	}
	if d.HasChange("subnet_ip") {
		ipcfg.SubnetIP = utils.String(d.Get("subnet_ip").(string))
	}
	if d.HasChange("dhcp_server_address") {
		dhcs := &v3.Address{}

		dh := d.Get("dhcp_server_address").(map[string]interface{})
		dhcs.IP = utils.String(dh["ip"].(string))
		dhcs.IPV6 = utils.String(dh["ipv6"].(string))
		dhcs.FQDN = utils.String(dh["fqdn"].(string))

		ipcfg.DHCPServerAddress = dhcs
	}
	if d.HasChange("dhcp_server_address_port") {
		ipcfg.DHCPServerAddress.Port = utils.Int64(int64(d.Get("dhcp_server_address_port").(int)))
	}
	if d.HasChange("vlan_id") {
		res.VlanID = utils.Int64(int64(d.Get("vlan_id").(int)))
	}

	ipcfg.DHCPOptions = dhcpO
	res.IPConfig = ipcfg
	spec.Resources = res
	request.Metadata = metadata
	request.Spec = spec

	utils.PrintToJSON(request, "UPDATE METHOD REQUEST")

	if _, errUpdate := conn.V3.UpdateSubnet(d.Id(), request); errUpdate != nil {
		return errUpdate
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PENDING", "RUNNING"},
		Target:     []string{"COMPLETE"},
		Refresh:    subnetStateRefreshFunc(conn, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for subnet (%s) to update: %s", d.Id(), err)
	}
	return resourceNutanixSubnetRead(d, meta)
}

func resourceNutanixSubnetDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*NutanixClient).API

	log.Printf("Destroying the subnet with the uuid %s", d.Id())
	fmt.Printf("Destroying the subnet with the uuid %s", d.Id())

	if err := conn.V3.DeleteSubnet(d.Id()); err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PENDING", "RUNNING", "DELETE_IN_PROGRESS", "COMPLETE"},
		Target:     []string{"DELETED"},
		Refresh:    subnetStateRefreshFunc(conn, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for subnet (%s) to delete: %s", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func resourceNutanixSubnetExists(conn *v3.Client, name string) (*string, error) {
	log.Printf("[DEBUG] Get Subnet Existence: %s", name)

	subnetEntities := &v3.SubnetListMetadata{}
	var subnetUUID *string

	subnetList, err := conn.V3.ListSubnet(subnetEntities)

	if err != nil {
		return nil, err
	}

	for _, subnet := range subnetList.Entities {
		if subnet.Status.Name == utils.String(name) {
			subnetUUID = subnet.Metadata.UUID
		}
	}
	return subnetUUID, nil
}

func getSubnetResources(d *schema.ResourceData, subnet *v3.SubnetResources) error {

	ip := &v3.IPConfig{}
	dhcpo := &v3.DHCPOptions{}

	if v, ok := d.GetOk("vswitch_name"); ok {
		subnet.VswitchName = utils.String(v.(string))
	}
	if st, ok := d.GetOk("subnet_type"); ok {
		subnet.SubnetType = utils.String(st.(string))
	}
	if v, ok := d.GetOk("default_gateway_ip"); ok {
		ip.DefaultGatewayIP = utils.String(v.(string))
	}
	if v, ok := d.GetOk("prefix_length"); ok {
		ip.PrefixLength = utils.Int64(int64(v.(int)))
	}
	if v, ok := d.GetOk("subnet_ip"); ok {
		ip.SubnetIP = utils.String(v.(string))
	}
	if v, ok := d.GetOk("dhcp_server_address"); ok {
		dhcpa := v.(map[string]interface{})
		address := &v3.Address{}

		if ip, ok := dhcpa["ip"]; ok {
			address.IP = utils.String(ip.(string))
		}
		if fqdn, ok := dhcpa["fqdn"]; ok {
			address.FQDN = utils.String(fqdn.(string))
		}
		if v, ok := d.GetOk("dhcp_server_address_port"); ok {
			address.Port = utils.Int64(int64(v.(int64)))
		}
		if ipv6, ok := dhcpa["ipv6"]; ok {
			address.IPV6 = utils.String(ipv6.(string))
		}

		ip.DHCPServerAddress = address
	}
	if v, ok := d.GetOk("ip_config_pool_list_ranges"); ok {
		p := v.([]interface{})
		pool := make([]*v3.IPPool, len(p))

		for k, v := range p {
			pItem := &v3.IPPool{}
			pItem.Range = utils.String(v.(string))
			pool[k] = pItem
		}

		ip.PoolList = pool
	}
	if v, ok := d.GetOk("dhcp_options"); ok {
		dop := v.(map[string]interface{})

		if boot, ok := dop["boot_file_name"]; ok {
			dhcpo.BootFileName = utils.String(boot.(string))
		}

		if dn, ok := dop["domain_name"]; ok {
			dhcpo.DomainName = utils.String(dn.(string))
		}

		if tsn, ok := dop["tftp_server_name"]; ok {
			dhcpo.TFTPServerName = utils.String(tsn.(string))
		}
	}

	if v, ok := d.GetOk("dhcp_domain_name_server_list"); ok {
		p := v.([]interface{})
		pool := make([]*string, len(p))

		for k, v := range p {
			pool[k] = utils.String(v.(string))
		}

		dhcpo.DomainNameServerList = pool
	}
	if v, ok := d.GetOk("dhcp_domain_search_list"); ok {
		p := v.([]interface{})
		pool := make([]*string, len(p))

		for k, v := range p {
			pool[k] = utils.String(v.(string))
		}

		dhcpo.DomainSearchList = pool
	}

	//set vlan_id
	v, ok := d.GetOk("vlan_id")
	if v.(int) == 0 || ok {
		subnet.VlanID = utils.Int64(int64(v.(int)))
	}

	// set network_function_chain_reference
	if v, ok := d.GetOk("network_function_chain_reference"); ok {
		ref := v.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(ref["kind"].(string)),
			UUID: utils.String(ref["uuid"].(string)),
		}
		if v, ok := ref["name"]; ok {
			r.Name = utils.String(v.(string))
		}
		subnet.NetworkFunctionChainReference = r
	}

	ip.DHCPOptions = dhcpo

	subnet.IPConfig = ip

	return nil
}

func getSubnetMetadaAttributes(d *schema.ResourceData, metadata *v3.SubnetMetadata) error {
	m, mok := d.GetOk("metadata")
	metad := m.(map[string]interface{})

	if !mok {
		return fmt.Errorf("please provide metadata required attributes")
	}

	metadata.Kind = utils.String(metad["kind"].(string))

	if v, ok := metad["uuid"]; ok && v != "" {
		metadata.UUID = utils.String(v.(string))
	}
	if v, ok := metad["spec_version"]; ok && v != 0 {
		i, err := strconv.Atoi(v.(string))
		if err != nil {
			return err
		}
		metadata.SpecVersion = utils.Int64(int64(i))
	}
	if v, ok := metad["spec_hash"]; ok && v != "" {
		metadata.SpecHash = utils.String(v.(string))
	}
	if v, ok := metad["name"]; ok {
		metadata.Name = utils.String(v.(string))
	}
	if v, ok := d.GetOk("categories"); ok {
		c := v.(map[string]interface{})
		labels := map[string]string{}

		for k, v := range c {
			labels[k] = v.(string)
		}
		metadata.Categories = labels
	}
	if p, ok := d.GetOk("project_reference"); ok {
		pr := p.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(pr["kind"].(string)),
			UUID: utils.String(pr["uuid"].(string)),
		}
		if v1, ok1 := pr["name"]; ok1 {
			r.Name = utils.String(v1.(string))
		}
		metadata.ProjectReference = r
	}
	if o, ok := metad["owner_reference"]; ok {
		or := o.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(or["kind"].(string)),
			UUID: utils.String(or["uuid"].(string)),
		}
		if v1, ok1 := or["name"]; ok1 {
			r.Name = utils.String(v1.(string))
		}
		metadata.OwnerReference = r
	}

	return nil
}

func setSubnetResources(m interface{}) (*v3.SubnetResources, error) {

	subnet := &v3.SubnetResources{}

	resources := m.(map[string]interface{})

	if v, ok := resources["vswitch_name"]; ok {
		subnet.VswitchName = utils.String(v.(string))
	}

	st, stok := resources["subnet_type"]

	if !stok {
		return nil, fmt.Errorf("Plase provide required subnet_type attribute")
	}

	if vlan, ok := resources["vlan_id"]; ok {
		if n, err := strconv.Atoi(vlan.(string)); err == nil {
			subnet.VlanID = utils.Int64(int64(n))
		}

	}

	nfcr, nfcrok := resources["network_function_chain_reference"]

	if nfcrok {
		a := nfcr.(map[string]interface{})
		r := &v3.Reference{
			Kind: utils.String(a["kind"].(string)),
			UUID: utils.String(a["uuid"].(string)),
		}
		if v, ok := a["name"]; ok {
			r.Name = utils.String(v.(string))
		}
		subnet.NetworkFunctionChainReference = r
	}

	//ip config
	if ipcfg, ipcok := resources["ip_config"]; ipcok {
		ipc := ipcfg.([]interface{})
		if len(ipc) > 0 {
			cfg := ipc[0].(map[string]interface{})
			ipConf := &v3.IPConfig{}

			if d, ok := cfg["default_gateway_ip"]; ok {
				ipConf.DefaultGatewayIP = utils.String(d.(string))
			}

			if d, ok := cfg["prefix_length"]; ok {
				ipConf.PrefixLength = utils.Int64(d.(int64))
			}

			if d, ok := cfg["subnet_ip"]; ok {
				ipConf.SubnetIP = utils.String(d.(string))
			}

			if dhcp, dok := cfg["dhcp_server_address"]; dok {
				dhcpa := dhcp.(map[string]interface{})
				address := &v3.Address{}

				if ip, ok := dhcpa["ip"]; ok {
					address.IP = utils.String(ip.(string))
				}

				if fqdn, ok := dhcpa["fqdn"]; ok {
					address.FQDN = utils.String(fqdn.(string))
				}

				if port, ok := dhcpa["port"]; ok {
					address.Port = utils.Int64(int64(port.(int64)))
				}

				if ipv6, ok := dhcpa["ipv6"]; ok {
					address.IPV6 = utils.String(ipv6.(string))
				}

				ipConf.DHCPServerAddress = address
			}

			if pl, ok := cfg["pool_list"]; ok {
				p := pl.([]map[string]interface{})

				pool := make([]*v3.IPPool, len(p))

				for k, v := range p {
					pItem := &v3.IPPool{}
					if val, ok := v["range"]; ok {
						pItem.Range = utils.String(val.(string))
					}
					pool[k] = pItem
				}

				ipConf.PoolList = pool
			}

			if do, ok := cfg["dhcp_options"]; ok {
				dhcpo := &v3.DHCPOptions{}

				dop := do.(map[string]interface{})

				if dn, ok := dop["domain_name_server_list"]; ok {
					dnsl := dn.([]*string)

					domainNameServerList := make([]*string, len(dnsl))

					for k, v := range dnsl {
						domainNameServerList[k] = v
					}

					dhcpo.DomainNameServerList = domainNameServerList
				}

				if boot, ok := dop["boot_file_name"]; ok {
					dhcpo.BootFileName = utils.String(boot.(string))
				}

				if ds, ok := dop["domain_search_list"]; ok {
					dsl := ds.([]*string)

					domainSearchList := make([]*string, len(dsl))

					for k, v := range dsl {
						domainSearchList[k] = v
					}

					dhcpo.DomainSearchList = domainSearchList
				}

				if dn, ok := dop["domain_name"]; ok {
					dhcpo.DomainName = utils.String(dn.(string))
				}

				if tsn, ok := dop["tftp_server_name"]; ok {
					dhcpo.TFTPServerName = utils.String(tsn.(string))
				}

				ipConf.DHCPOptions = dhcpo
			}
			subnet.IPConfig = ipConf
		}

	}

	subnet.SubnetType = utils.String(st.(string))

	return subnet, nil
}

func setSubnetResourcesIPConfig(ic interface{}) *v3.IPConfig {
	cfg := ic.(map[string]interface{})

	ipConf := &v3.IPConfig{}

	if d, ok := cfg["default_gateway_ip"]; ok {
		ipConf.DefaultGatewayIP = utils.String(d.(string))
	}

	if d, ok := cfg["prefix_length"]; ok {
		if n, err := strconv.Atoi(d.(string)); err == nil {
			ipConf.PrefixLength = utils.Int64(int64(n))
		}

	}

	if d, ok := cfg["subnet_ip"]; ok {
		ipConf.SubnetIP = utils.String(d.(string))
	}

	if dhcp, dok := cfg["dhcp_server_address"]; dok {
		dhcpa := dhcp.(map[string]interface{})
		address := &v3.Address{}

		if ip, ok := dhcpa["ip"]; ok {
			address.IP = utils.String(ip.(string))
		}

		if fqdn, ok := dhcpa["fqdn"]; ok {
			address.FQDN = utils.String(fqdn.(string))
		}

		if port, ok := dhcpa["port"]; ok {
			address.Port = utils.Int64(int64(port.(int64)))
		}

		if ipv6, ok := dhcpa["ipv6"]; ok {
			address.IPV6 = utils.String(ipv6.(string))
		}

		ipConf.DHCPServerAddress = address
	}
	return ipConf
}

func setSubnetResourcesDHCPOptions(dhcp interface{}) *v3.DHCPOptions {
	dhcpo := &v3.DHCPOptions{}

	dop := dhcp.(map[string]interface{})

	if boot, ok := dop["boot_file_name"]; ok {
		dhcpo.BootFileName = utils.String(boot.(string))
	}

	if dn, ok := dop["domain_name"]; ok {
		dhcpo.DomainName = utils.String(dn.(string))
	}

	if tsn, ok := dop["tftp_server_name"]; ok {
		dhcpo.TFTPServerName = utils.String(tsn.(string))
	}

	return dhcpo

}

func setSubnetMetadata(m interface{}) *v3.SubnetMetadata {
	metad := m.(map[string]interface{})
	metadata := &v3.SubnetMetadata{
		Kind: utils.String(metad["kind"].(string)),
	}
	if v, ok := metad["uuid"]; ok {
		metadata.UUID = utils.String(v.(string))
	}
	if v, ok := metad["spec_version"]; ok {
		if n, err := strconv.Atoi(v.(string)); err == nil {
			metadata.SpecVersion = utils.Int64(int64(n))
		}
	}
	if v, ok := metad["spec_hash"]; ok {
		metadata.SpecHash = utils.String(v.(string))
	}
	if v, ok := metad["name"]; ok {
		metadata.Name = utils.String(v.(string))
	}
	if v, ok := metad["categories"]; ok {
		metadata.Categories = v.(map[string]string)
	}

	return metadata
}

func subnetStateRefreshFunc(client *v3.Client, uuid string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		v, err := client.V3.GetSubnet(uuid)

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "ENTITY_NOT_FOUND") {
				return v, "DELETED", nil
			}
			log.Printf("ERROR %s", err)
			return nil, "", err
		}

		return v, *v.Status.State, nil
	}
}

func getSubnetSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"api_version": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"metadata": {
			Type:     schema.TypeMap,
			Required: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"last_update_time": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"kind": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"creation_time": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"spec_version": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"spec_hash": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"categories": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
		},
		"owner_reference": {
			Type:     schema.TypeMap,
			Optional: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"kind": {
						Type:     schema.TypeString,
						Optional: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Optional: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
					},
				},
			},
		},
		"project_reference": {
			Type:     schema.TypeMap,
			Optional: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"kind": {
						Type:     schema.TypeString,
						Optional: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Optional: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
					},
				},
			},
		},
		"name": {
			Type:     schema.TypeString,
			Required: true,
		},
		"state": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"description": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"availability_zone_reference": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"kind": {
						Type:     schema.TypeString,
						Required: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Required: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"cluster_reference": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"kind": {
						Type:     schema.TypeString,
						Required: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Required: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"vswitch_name": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"subnet_type": {
			Type:     schema.TypeString,
			Required: true,
		},
		"default_gateway_ip": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"prefix_length": {
			Type:     schema.TypeInt,
			Optional: true,
			Computed: true,
		},
		"subnet_ip": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
		"dhcp_server_address": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"ip": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"fqdn": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"ipv6": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"dhcp_server_address_port": {
			Type:     schema.TypeInt,
			Optional: true,
			Computed: true,
		},
		"ip_config_pool_list_ranges": {
			Type:     schema.TypeList,
			Optional: true,
			Computed: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
		"dhcp_options": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"boot_file_name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"domain_name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"tftp_server_name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"dhcp_domain_name_server_list": {
			Type:     schema.TypeList,
			Optional: true,
			Computed: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
		"dhcp_domain_search_list": {
			Type:     schema.TypeList,
			Optional: true,
			Computed: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
		"vlan_id": {
			Type:     schema.TypeInt,
			Optional: true,
			ForceNew: true,
			Computed: true,
		},
		"network_function_chain_reference": {
			Type:     schema.TypeMap,
			Optional: true,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"kind": {
						Type:     schema.TypeString,
						Required: true,
					},
					"uuid": {
						Type:     schema.TypeString,
						Required: true,
					},
					"name": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
	}
}
