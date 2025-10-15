# S3 Certification Process Requirements

> **OpenShift API for Data Protection (OADP)**  
> S3-Compatible Object Storage Certification Guide

---

## 📋 Table of Contents

- [Overview](#overview)
- [I. Access and Environment Requirements](#i-access-and-environment-requirements)
- [II. Support and Communication](#ii-support-and-communication)
- [III. Certification Process and Timeline](#iii-certification-process-and-timeline)
- [Quick Checklist](#quick-checklist)
- [Getting Started](#getting-started)
- [Contact Information](#contact-information)

---

## Overview

This document outlines the general requirements, access needs, and steps involved in certifying an **S3-compatible object storage product** for use with **OADP (OpenShift API for Data Protection)**.

> ⚠️ **Important**: This certification process ensures your S3-compatible storage solution meets the reliability and compatibility standards required for production OpenShift backup and restore operations.

### 📋 **Currently Supported S3-Compatible Providers**

OADP currently supports the following S3-compatible storage solutions:

📖 **[View Currently Supported S3-Compatible Storage Providers](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/backup_and_restore/oadp-application-backup-and-restore#oadp-s3-compatible-backup-storage-providers_about-installing-oadp)** - Official Red Hat documentation listing all certified providers.

---

## I. Access and Environment Requirements

### 🪣 1. S3 Bucket Access

- **Dedicated Bucket**: Provision an S3 bucket dedicated to certification testing
- **Minimum Capacity**: The bucket should have a minimum capacity of **10GB** to accommodate test workloads
- **Configuration Access**: Ensure the ability to modify bucket settings, including features such as:
  - Object locking
  - Versioning
  - Lifecycle policies
  - Access control

### 🔐 2. Credentials

#### UI Access
- Provide credentials (**username** and **password**) for accessing the product's management interface

#### CLI Access  
- Provide an **Access Key** and **Secret Key** for use with CLI-based testing
- Ensure keys have sufficient permissions for backup/restore operations

> 🔒 **Security Note**: All credentials will be handled securely and used only for certification testing. 

### 🌐 3. S3 Endpoint

- **Stable Endpoint**: A stable and reachable S3 endpoint URL must be provided
  ```
  Example: https://s3.your-storage.com
  ```
- **Fixed IP Resolution**: The endpoint should resolve to a fixed IP address
  
### 🔒 4. SSL Certificates

- **SSL Verification**: If SSL verification is enforced, provide:
  - Certificate installation procedure **OR**
  - Root certificate installation steps needed to establish trust for the endpoint

---

## II. Support and Communication

### 💬 Support Channel
- Establish a clear communication channel for addressing technical questions and issues during testing:
  - **Email** support channel
  - **Slack** workspace access
  - **Other** preferred communication method

### 👥 Points of Contact

| Role | Responsibility |
|------|----------------|
| **Technical Contact** | Configuration and debugging assistance |
| **Support Contact** | Escalations or environment issues |

---

## III. Certification Process and Timeline

### ⏱️ Duration
- **Estimated Timeline**: The certification process typically completes within **3–4 weeks**
- **Dependencies**: Timeline depends on:
  - Environment readiness
  - Issue turnaround time
  - Response time for technical queries

### 🎯 Certification Outcome
Upon successful validation, the product will be **listed as a supported S3-compatible backup storage provider** in the official OADP documentation:

📖 **[S3-Compatible Backup Storage Providers - OpenShift Container Platform Documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/backup_and_restore/oadp-application-backup-and-restore#oadp-certified-backup-storage-providers_about-installing-oadp)**

This inclusion provides:
- **Official Red Hat support recognition**
- **Customer confidence** in your storage solution
- **Technical validation** for enterprise deployments
- **Integration guidance** for OpenShift users

### 🔗 Endpoint Availability

#### Temporary Endpoints
- **Current Testing**: Temporary endpoints (valid for a few weeks) are sufficient for testing current OADP releases

#### Permanent Endpoints *(Recommended)*
- **Future Validation**: For ongoing validation and future OADP versions.
- **Requirement**: Permanent and stable endpoint preferred


## 📝 Quick Checklist

Use this checklist to ensure all requirements are met before starting certification:

- [ ] **S3 Bucket**: 10GB+ dedicated bucket provisioned
- [ ] **UI Credentials**: Username/password for management interface
- [ ] **CLI Credentials**: Access Key/Secret Key provided  
- [ ] **Endpoint**: Stable S3 endpoint URL available
- [ ] **SSL**: Certificate trust established (if required)
- [ ] **Support Channel**: Communication method established
- [ ] **Contacts**: Technical and support contacts identified
- [ ] **Timeline**: 3-4 week certification window confirmed

---

## 🤝 Getting Started

To initiate the S3 certification process:

1. **Review Requirements**: Ensure all items in the checklist above are completed
2. **Contact OADP Team**: Reach out to begin the certification process
3. **Environment Setup**: Provide access credentials and endpoint information
4. **Testing Phase**: Collaborate during the 3-4 week testing period
5. **Documentation**: Upon success, your product will be added to supported providers

---

## 📞 Contact Information

For questions or to begin the S3 certification process:

- **OADP Team**: [akarol@redhat.com](mailto:akarol@redhat.com),[whayutin@redhat.com](mailto:whayutin@redhat.com)
- **GitHub Issues**: [OADP Operator Repository](https://github.com/openshift/oadp-operator/issues)

---

> 📄 **Document Version**: 1.0  
> **Last Updated**: October 7, 2025  


